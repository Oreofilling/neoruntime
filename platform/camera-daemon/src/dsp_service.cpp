/**
 * @file dsp_service.cpp
 * @brief Implementation of the app-facing DSP job service (PLAT-1/4/5).
 *
 * Locking model (independent guards; buffers_mu_ is NEVER held while
 * calling into HAL — allocs/frees run on collected lists after unlock,
 * and unpin_entries takes buffers_mu_ itself):
 *  - buffers_mu_ : registry maps + BufferEntry pin/detach state
 *  - q_mu_       : job deques            - done_mu_  : job done/abandoned
 *  - quota_mu_   : token buckets         - stats_mu_ : counters
 *
 * CPU coherency: the daemon never CPU-touches registered buffers, so it
 * does no DMA_BUF_IOCTL_SYNC itself. The DMA_BUF_IOCTL_SYNC discipline
 * (write-fence after CPU fill, read-fence before CPU read) is part of the
 * client-side contract — see docs/proposals/dsp-offload.md (HAL-3).
 */

#include "dsp_service.h"

#include <algorithm>
#include <cstdio>
#include <memory>
#include <utility>
#include <unistd.h> /* dup, close */

#include "common/hal_log.h"

namespace {

constexpr uint32_t kMinDim = 16;
constexpr uint32_t kMaxDim = 8192;
/* SCM_RIGHTS wire cap on the UDS alloc response: count*num_planes fds. */
constexpr uint32_t kMaxAllocFds = 64;

bool format_supported(HalPixelFormat f) {
    switch (f) {
    case HAL_PIX_FMT_NV12:
    case HAL_PIX_FMT_RGB24:
    case HAL_PIX_FMT_GRAY8:
        return true;
    default:
        return false;
    }
}

/* Plane count for the P0 format set (mirrors hal_pixel_format_plane_count,
 * which lives in hal_v2/common/hal_buffer.c — not linked into the daemon). */
uint32_t plane_count_of(HalPixelFormat f) {
    return (f == HAL_PIX_FMT_NV12) ? 2 : 1;
}

/* Minimum sane stride for plane p of a w*h `format` buffer (no padding). */
uint32_t min_stride_of(HalPixelFormat f, uint32_t w) {
    if (f == HAL_PIX_FMT_RGB24) return w * 3;
    return w; /* NV12: Y row and interleaved-UV row are both w bytes */
}

/* Rows in plane p (NV12 chroma is half height). */
uint32_t plane_rows_of(HalPixelFormat f, uint32_t h, uint32_t plane) {
    if (f == HAL_PIX_FMT_NV12 && plane == 1) return h / 2;
    return h;
}

/* Imported descriptors are plain daemon-owned allocations, not HAL pool
 * buffers: releasing one is close(dup'd fds) + delete. They must never
 * reach fb_ops_->release_frame_buffer. */
void free_imported_fb(HalFrameBuffer* fb) {
    if (!fb) return;
    for (uint32_t p = 0; p < HAL_MAX_PLANES; ++p)
        if (fb->dma_fds[p] >= 0) close(fb->dma_fds[p]);
    delete fb;
}

} // namespace

DspService::DspService(HalDspOps* dsp_ops, HalFrameBufferOps* fb_ops,
                       const DspServiceConfig& cfg)
    : dsp_ops_(dsp_ops), fb_ops_(fb_ops), cfg_(cfg) {}

DspService::~DspService() { stop(); }

/* ------------------------------------------------------------------ */
/* Lifecycle                                                           */
/* ------------------------------------------------------------------ */

bool DspService::start() {
    if (running_.load()) return true;
    if (!dsp_ops_ || !fb_ops_) {
        HAL_LOG_ERROR("DspService: null ops table (dsp=%p fb=%p)", dsp_ops_,
                      fb_ops_);
        return false;
    }

    // Same context recipe as dpm_worker (E1 proved contexts coexist; the
    // vendor PriorityQueueSingleton serializes hardware access anyway).
    HalDspConfig dcfg{};
    dcfg.device_priority = 0;
    if (dsp_ops_->init(&dcfg, &dsp_ctx_) != 0 || !dsp_ctx_) {
        HAL_LOG_ERROR("DspService: HAL DSP init failed — SubmitDspJob unavailable");
        dsp_ctx_ = nullptr;
        return false;
    }

    running_ = true;
    worker_ = std::thread(&DspService::worker_loop, this);
    HAL_LOG_INFO(
        "DspService: started (max_batch=%u quota=%.0f jobs/s %.0f MPix/s "
        "timeout=%ums)",
        cfg_.max_batch, cfg_.quota_jobs_per_sec, cfg_.quota_mpix_per_sec,
        cfg_.job_timeout_ms);
    return true;
}

void DspService::stop() {
    if (!running_.exchange(false)) {
        if (worker_.joinable()) worker_.join(); // never fully started
        return;
    }
    q_cv_.notify_all();
    if (worker_.joinable()) worker_.join();

    // Fail submitters still waiting on queued jobs, then drop their pins.
    std::vector<JobRef> leftover;
    {
        std::lock_guard<std::mutex> lk(q_mu_);
        for (JobRef& j : q_normal_) leftover.push_back(std::move(j));
        for (JobRef& j : q_background_) leftover.push_back(std::move(j));
        q_normal_.clear();
        q_background_.clear();
    }
    for (auto& job : leftover) {
        job->result.rc = DSP_SVC_ERR_UNAVAILABLE;
        job->result.message = "service stopping";
        unpin_entries(job->pinned);
        job->pinned.clear();
        {
            std::lock_guard<std::mutex> lk(done_mu_);
            job->done = true;
        }
        done_cv_.notify_all();
    }

    // Free every remaining registered buffer (no pins can exist now).
    {
        std::vector<HalFrameBuffer*> to_free;
        std::vector<HalFrameBuffer*> imported;
        {
            std::lock_guard<std::mutex> lk(buffers_mu_);
            to_free.reserve(buffers_.size());
            for (auto& kv : buffers_) {
                if (kv.second->imported) imported.push_back(kv.second->fb);
                else to_free.push_back(kv.second->fb);
                delete kv.second;
            }
            buffers_.clear();
            client_buffer_count_.clear();
            client_pixels_.clear();
            client_import_count_.clear();
        }
        for (HalFrameBuffer* fb : to_free) fb_ops_->release_frame_buffer(fb);
        for (HalFrameBuffer* fb : imported) free_imported_fb(fb);
    }

    if (dsp_ctx_) {
        dsp_ops_->deinit(dsp_ctx_);
        dsp_ctx_ = nullptr;
    }
    HAL_LOG_INFO("DspService: stopped");
}

/* ------------------------------------------------------------------ */
/* Buffer plane                                                        */
/* ------------------------------------------------------------------ */

DspService::AllocResult DspService::alloc_buffers(int client_fd, uint32_t width,
                                                  uint32_t height,
                                                  HalPixelFormat format,
                                                  uint32_t count) {
    AllocResult out;
    if (!running_.load() || !fb_ops_) {
        out.rc = DSP_SVC_ERR_UNAVAILABLE;
        out.message = "service not running";
        return out;
    }
    if (count == 0) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "count must be >= 1";
        return out;
    }
    if (width < kMinDim || height < kMinDim || width > kMaxDim ||
        height > kMaxDim) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "width/height out of range [16, 8192]";
        return out;
    }
    if (!format_supported(format)) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "unsupported format (NV12/RGB24/GRAY8 in P0)";
        return out;
    }
    const uint64_t px = pixels_of(width, height);
    if (px > cfg_.max_pixels_per_op) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "buffer exceeds max_pixels_per_op";
        return out;
    }
    uint32_t planes = plane_count_of(format);
    if (static_cast<uint64_t>(count) * planes > kMaxAllocFds) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "count*num_planes exceeds the 64-fd UDS response cap";
        return out;
    }

    // Fail fast on per-client caps (re-checked atomically after alloc).
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        uint32_t have_n = client_buffer_count_[client_fd];
        uint64_t have_px = client_pixels_[client_fd];
        if (have_n + count > cfg_.max_buffers_per_client) {
            out.rc = DSP_SVC_ERR_LIMIT;
            out.message = "per-client buffer count cap exceeded";
            return out;
        }
        if (have_px + px * count > cfg_.max_client_pixels) {
            out.rc = DSP_SVC_ERR_LIMIT;
            out.message = "per-client outstanding pixel cap exceeded";
            return out;
        }
    }

    // HAL allocs outside the registry lock (can be slow).
    HalFrameBufferRequest req{};
    req.width = width;
    req.height = height;
    req.format = format;
    // hal_v2's default app pool is 8 buffers/geometry (hailo15_media_impl.cpp
    // kDefaultMaxBuffers) — too small for MULTI_CROP batches. Size the pool to
    // the fd-plane ceiling: 32 two-plane buffers = FD_PUB_DSP_MAX_FDS(64) fds.
    // Pool key includes the size, so this never touches the pipeline's own
    // pool_max_buffers=0 pools.
    req.pool_max_buffers = 32;
    req.mem_type = HAL_MEM_DMABUF;
    req.zero_initialize = false;

    std::vector<HalFrameBuffer*> fbs;
    fbs.reserve(count);
    for (uint32_t i = 0; i < count; ++i) {
        HalFrameBuffer* fb = nullptr;
        int rc = fb_ops_->request_frame_buffer(&req, &fb);
        if (rc != 0 || !fb) {
            for (HalFrameBuffer* done : fbs) fb_ops_->release_frame_buffer(done);
            out.rc = DSP_SVC_ERR_NO_MEM;
            char msg[128];
            std::snprintf(msg, sizeof(msg), "HAL alloc failed at %u/%u (rc=%d)", i,
                          count, rc);
            out.message = msg;
            HAL_LOG_WARNING("DspService: %s", msg);
            return out;
        }
        fbs.push_back(fb);
    }

    // Register atomically.
    std::vector<HalFrameBuffer*> rollback;
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        uint32_t have_n = client_buffer_count_[client_fd];
        uint64_t have_px = client_pixels_[client_fd];
        if (have_n + count > cfg_.max_buffers_per_client ||
            have_px + px * count > cfg_.max_client_pixels) {
            rollback = std::move(fbs);
        } else {
            client_buffer_count_[client_fd] = have_n + count;
            client_pixels_[client_fd] = have_px + px * count;
            out.num_planes = fbs[0]->num_planes;
            for (uint32_t p = 0; p < HAL_MAX_PLANES; ++p) {
                out.strides[p] = fbs[0]->strides[p];
                out.sizes[p] = fbs[0]->sizes[p];
            }
            out.ids.reserve(count);
            out.fds.reserve(static_cast<size_t>(count) * out.num_planes);
            for (HalFrameBuffer* fb : fbs) {
                auto* e = new BufferEntry();
                e->id = next_buffer_id_++;
                e->client_fd = client_fd;
                e->fb = fb;
                buffers_[e->id] = e;
                out.ids.push_back(e->id);
                for (uint32_t p = 0; p < out.num_planes; ++p)
                    out.fds.push_back(fb->dma_fds[p]);
            }
        }
    }
    if (!rollback.empty()) {
        for (HalFrameBuffer* fb : rollback) fb_ops_->release_frame_buffer(fb);
        out.rc = DSP_SVC_ERR_LIMIT;
        out.message = "per-client cap exceeded racing another alloc";
        return out;
    }

    {
        std::lock_guard<std::mutex> lk(stats_mu_);
        stats_.buffers_allocated += count;
        stats_.buffers_in_registry += count;
    }
    return out;
}

DspService::ImportResult DspService::import_buffer(
    int client_fd, uint32_t width, uint32_t height, HalPixelFormat format,
    uint32_t num_planes, const uint32_t* strides, const uint32_t* sizes,
    const int* fds) {
    ImportResult out;
    if (!running_.load() || !fb_ops_) {
        out.rc = DSP_SVC_ERR_UNAVAILABLE;
        out.message = "service not running";
        return out;
    }
    if (width < kMinDim || height < kMinDim || width > kMaxDim ||
        height > kMaxDim) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "width/height out of range [16, 8192]";
        return out;
    }
    if (!format_supported(format)) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "unsupported format (NV12/RGB24/GRAY8 in P0)";
        return out;
    }
    if (pixels_of(width, height) > cfg_.max_pixels_per_op) {
        out.rc = DSP_SVC_ERR_INVALID;
        out.message = "buffer exceeds max_pixels_per_op";
        return out;
    }
    const uint32_t planes = plane_count_of(format);
    if (num_planes != planes) {
        out.rc = DSP_SVC_ERR_INVALID;
        char msg[96];
        std::snprintf(msg, sizeof(msg),
                      "num_planes %u does not match format (%u expected)",
                      num_planes, planes);
        out.message = msg;
        return out;
    }
    for (uint32_t p = 0; p < planes; ++p) {
        const uint32_t rows = plane_rows_of(format, height, p);
        if (strides[p] < min_stride_of(format, width) ||
            sizes[p] < static_cast<uint64_t>(strides[p]) * rows) {
            out.rc = DSP_SVC_ERR_INVALID;
            char msg[128];
            std::snprintf(msg, sizeof(msg),
                          "plane %u geometry implausible (stride %u, size %u)",
                          p, strides[p], sizes[p]);
            out.message = msg;
            return out;
        }
    }

    // Dup outside the registry lock (syscalls). The daemon keeps its own fd
    // copies, so the client may close theirs immediately if it wants.
    int dup_fds[HAL_MAX_PLANES] = {-1, -1, -1};
    for (uint32_t p = 0; p < planes; ++p) {
        dup_fds[p] = dup(fds[p]);
        if (dup_fds[p] < 0) {
            for (uint32_t q = 0; q < p; ++q) close(dup_fds[q]);
            out.rc = DSP_SVC_ERR_NO_MEM;
            out.message = "dup of client dma-buf fd failed";
            return out;
        }
    }

    // A plain descriptor the DSP HAL reads like any pool buffer: geometry +
    // fds + strides. hal_frame_to_dsp_image() only consumes these fields
    // (hailo15_dsp_impl.cpp) — refcounts/priv belong to HAL pool buffers and
    // are deliberately left zero.
    HalFrameBuffer* fb = new HalFrameBuffer();
    fb->width = width;
    fb->height = height;
    fb->format = format;
    fb->mem_type = HAL_MEM_DMABUF;
    fb->num_planes = planes;
    for (uint32_t p = 0; p < HAL_MAX_PLANES; ++p) {
        fb->dma_fds[p] = (p < planes) ? dup_fds[p] : -1;
        fb->strides[p] = (p < planes) ? strides[p] : 0;
        fb->sizes[p] = (p < planes) ? sizes[p] : 0;
    }

    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        uint32_t have = client_import_count_[client_fd];
        if (have + 1 > cfg_.max_imports_per_client) {
            free_imported_fb(fb);
            out.rc = DSP_SVC_ERR_LIMIT;
            out.message = "per-client import cap exceeded";
            return out;
        }
        client_import_count_[client_fd] = have + 1;
        auto* e = new BufferEntry();
        e->id = next_buffer_id_++;
        e->client_fd = client_fd;
        e->fb = fb;
        e->imported = true;
        buffers_[e->id] = e;
        out.id = e->id;
    }

    {
        std::lock_guard<std::mutex> lk(stats_mu_);
        stats_.buffers_allocated++;
        stats_.buffers_in_registry++;
    }
    return out;
}

void DspService::detach_entry_locked(BufferEntry* entry,
                                     std::vector<HalFrameBuffer*>& to_free) {
    if (entry->detached) return;
    entry->detached = true;
    buffers_.erase(entry->id);
    if (entry->imported) {
        uint32_t n = client_import_count_[entry->client_fd];
        client_import_count_[entry->client_fd] = (n > 0) ? n - 1 : 0;
        {
            std::lock_guard<std::mutex> lk(stats_mu_);
            stats_.buffers_released++;
            if (stats_.buffers_in_registry > 0) stats_.buffers_in_registry--;
        }
        if (entry->pins == 0) {
            free_imported_fb(entry->fb);
            delete entry;
        }
        return;
    }
    uint32_t n = client_buffer_count_[entry->client_fd];
    client_buffer_count_[entry->client_fd] = (n > 0) ? n - 1 : 0;
    const uint64_t px = client_pixels_[entry->client_fd];
    const uint64_t sub = pixels_of(entry->fb->width, entry->fb->height);
    client_pixels_[entry->client_fd] = (px > sub) ? px - sub : 0;
    {
        std::lock_guard<std::mutex> lk(stats_mu_);
        stats_.buffers_released++;
        if (stats_.buffers_in_registry > 0) stats_.buffers_in_registry--;
    }
    if (entry->pins == 0) {
        to_free.push_back(entry->fb);
        delete entry;
    }
}

int DspService::release_buffer(int client_fd, uint64_t buffer_id) {
    std::vector<HalFrameBuffer*> to_free;
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        auto it = buffers_.find(buffer_id);
        if (it == buffers_.end()) return DSP_SVC_ERR_NO_BUFFER;
        if (it->second->client_fd != client_fd) return DSP_SVC_ERR_NO_BUFFER;
        detach_entry_locked(it->second, to_free);
    }
    for (HalFrameBuffer* fb : to_free) fb_ops_->release_frame_buffer(fb);
    return DSP_SVC_OK;
}

void DspService::release_client_buffers(int client_fd) {
    std::vector<HalFrameBuffer*> to_free;
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        for (auto it = buffers_.begin(); it != buffers_.end();) {
            if (it->second->client_fd == client_fd) {
                // detach_entry_locked erases `it` from the map.
                detach_entry_locked(it->second, to_free);
                it = buffers_.begin();
            } else {
                ++it;
            }
        }
        client_buffer_count_.erase(client_fd);
        client_pixels_.erase(client_fd);
        client_import_count_.erase(client_fd);
    }
    for (HalFrameBuffer* fb : to_free) fb_ops_->release_frame_buffer(fb);
    if (!to_free.empty())
        HAL_LOG_INFO("DspService: client %d disconnected, freed %zu buffer(s)",
                     client_fd, to_free.size());
    quota_forget(client_fd);
}

/* ------------------------------------------------------------------ */
/* Job plane                                                           */
/* ------------------------------------------------------------------ */

bool DspService::resolve_pin_buffer(uint64_t id, int& owner_fd_out,
                                    BufferEntry*& entry) {
    // Caller holds buffers_mu_.
    auto it = buffers_.find(id);
    if (it == buffers_.end()) return false;
    entry = it->second;
    owner_fd_out = entry->client_fd;
    entry->pins++;
    return true;
}

void DspService::unpin_entries(const std::vector<BufferEntry*>& entries) {
    if (entries.empty()) return;
    std::vector<HalFrameBuffer*> to_free;
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);
        for (BufferEntry* e : entries) {
            if (e->pins > 0) e->pins--;
            if (e->detached && e->pins == 0) {
                if (e->imported) free_imported_fb(e->fb);
                else to_free.push_back(e->fb);
                delete e;
            }
        }
    }
    for (HalFrameBuffer* fb : to_free) fb_ops_->release_frame_buffer(fb);
}

int DspService::validate_and_pin(DspJobDesc desc, JobRef& job_out,
                                 std::string& why) {
    if (desc.interpolation < 0 ||
        desc.interpolation >= HAL_DSP_INTERPOLATION_MAX) {
        why = "invalid interpolation";
        return DSP_SVC_ERR_INVALID;
    }
    if (desc.scaling_mode < 0 || desc.scaling_mode >= HAL_DSP_SCALING_MAX) {
        why = "invalid scaling_mode";
        return DSP_SVC_ERR_INVALID;
    }

    const bool wants_rects = desc.op == HAL_DSP_OP_CROP_RESIZE ||
                             desc.op == HAL_DSP_OP_MULTI_CROP_RESIZE;
    if (wants_rects && desc.rects.empty()) {
        why = "op requires >= 1 rect";
        return DSP_SVC_ERR_INVALID;
    }
    if (!wants_rects && !desc.rects.empty()) {
        why = "op does not take rects";
        return DSP_SVC_ERR_INVALID;
    }
    if (desc.dst_ids.empty()) {
        why = "no dst buffers";
        return DSP_SVC_ERR_INVALID;
    }
    if (desc.op == HAL_DSP_OP_MULTI_CROP_RESIZE &&
        (desc.dst_ids.size() > cfg_.max_batch ||
         desc.rects.size() != desc.dst_ids.size())) {
        why = "MULTI_CROP requires dst_ids.size() == rects.size() <= max_batch";
        return DSP_SVC_ERR_INVALID;
    }
    if (desc.op != HAL_DSP_OP_MULTI_CROP_RESIZE && desc.dst_ids.size() != 1) {
        why = "op requires exactly 1 dst buffer";
        return DSP_SVC_ERR_INVALID;
    }
    if (desc.op != HAL_DSP_OP_RESIZE && desc.op != HAL_DSP_OP_CROP_RESIZE &&
        desc.op != HAL_DSP_OP_MULTI_CROP_RESIZE &&
        desc.op != HAL_DSP_OP_CONVERT_FORMAT) {
        why = "op not available in P0";
        return DSP_SVC_ERR_INVALID;
    }

    auto job = std::make_shared<JobItem>();
    job->desc = std::move(desc);
    job->priority = job->desc.priority;

    // Locked section: resolve + pin every referenced buffer. On failure the
    // caller unpins AFTER the lock is gone (unpin_entries takes buffers_mu_).
    int vrc = DSP_SVC_OK;
    int owner_fd = -1;
    {
        std::lock_guard<std::mutex> lk(buffers_mu_);

        BufferEntry* src = nullptr;
        int src_owner = -1;
        if (!resolve_pin_buffer(job->desc.src_id, src_owner, src)) {
            vrc = DSP_SVC_ERR_NO_BUFFER;
            why = "src buffer not found";
        } else {
            owner_fd = src_owner;
            job->pinned.push_back(src);
            const HalFrameBuffer* sfb = src->fb;
            uint64_t dst_px_sum = 0;

            for (size_t i = 0; vrc == DSP_SVC_OK && i < job->desc.dst_ids.size();
                 ++i) {
                int dst_owner = -1;
                BufferEntry* dst = nullptr;
                if (!resolve_pin_buffer(job->desc.dst_ids[i], dst_owner, dst)) {
                    vrc = DSP_SVC_ERR_NO_BUFFER;
                    why = "dst buffer not found";
                    break;
                }
                job->pinned.push_back(dst);
                const HalFrameBuffer* dfb = dst->fb;

                if (dst->imported) {
                    vrc = DSP_SVC_ERR_INVALID;
                    why = "imported buffers are source-only (P0)";
                    break;
                }
                if (dfb->format != sfb->format &&
                    job->desc.op != HAL_DSP_OP_CONVERT_FORMAT) {
                    vrc = DSP_SVC_ERR_INVALID;
                    why = "src/dst format mismatch (only CONVERT_FORMAT allows it)";
                    break;
                }
                if (dfb->format == sfb->format &&
                    job->desc.op == HAL_DSP_OP_CONVERT_FORMAT) {
                    vrc = DSP_SVC_ERR_INVALID;
                    why = "CONVERT_FORMAT requires differing formats";
                    break;
                }

                if (wants_rects) {
                    const DspRect& r = job->desc.rects[i];
                    if (r.width == 0 || r.height == 0 || r.dst_width == 0 ||
                        r.dst_height == 0) {
                        vrc = DSP_SVC_ERR_INVALID;
                        why = "rect has zero dimension";
                        break;
                    }
                    if (r.x > sfb->width || r.y > sfb->height ||
                        r.width > sfb->width - r.x ||
                        r.height > sfb->height - r.y) {
                        vrc = DSP_SVC_ERR_INVALID;
                        why = "rect exceeds source bounds";
                        break;
                    }
                    if (r.dst_width != dfb->width || r.dst_height != dfb->height) {
                        vrc = DSP_SVC_ERR_INVALID;
                        why = "rect dst dims must match dst buffer dims";
                        break;
                    }
                } else if (job->desc.op == HAL_DSP_OP_CONVERT_FORMAT &&
                           (dfb->width != sfb->width ||
                            dfb->height != sfb->height)) {
                    vrc = DSP_SVC_ERR_INVALID;
                    why = "CONVERT_FORMAT requires equal dims (P0)";
                    break;
                }
                dst_px_sum += pixels_of(dfb->width, dfb->height);
            }

            if (vrc == DSP_SVC_OK) {
                const uint64_t src_px = pixels_of(sfb->width, sfb->height);
                if (src_px > cfg_.max_pixels_per_op ||
                    dst_px_sum > cfg_.max_pixels_per_op) {
                    vrc = DSP_SVC_ERR_INVALID;
                    why = "op exceeds max_pixels_per_op";
                } else {
                    job->charge_mpix =
                        static_cast<double>(src_px + dst_px_sum) / 1e6;
                }
            }
        }
    } // buffers_mu_ released

    if (vrc != DSP_SVC_OK) {
        unpin_entries(job->pinned);
        return vrc;
    }
    job->owner_fd = owner_fd;
    job_out = std::move(job);
    return DSP_SVC_OK;
}

bool DspService::quota_try_consume(int owner_fd, double mpix, std::string& why) {
    using clock = std::chrono::steady_clock;
    std::lock_guard<std::mutex> lk(quota_mu_);
    QuotaBucket& b = quotas_[owner_fd];
    const auto now = clock::now();
    const bool first_use = b.last.time_since_epoch().count() == 0;
    double dt = first_use ? 0.0
                          : std::chrono::duration<double>(now - b.last).count();
    if (dt < 0) dt = 0;
    b.last = now;
    if (first_use) {
        /* New owner: grant the full 1 s burst up front, so an app's very
         * first job is not rejected (burst = 1 s worth of budget). */
        b.jobs = cfg_.quota_jobs_per_sec;
        b.mpix = cfg_.quota_mpix_per_sec;
    } else {
        b.jobs = std::min(b.jobs + dt * cfg_.quota_jobs_per_sec,
                          cfg_.quota_jobs_per_sec);
        b.mpix = std::min(b.mpix + dt * cfg_.quota_mpix_per_sec,
                          cfg_.quota_mpix_per_sec);
    }
    if (b.jobs < 1.0) {
        char msg[96];
        std::snprintf(msg, sizeof(msg), "quota: jobs/s budget exhausted (%.0f/s)",
                      cfg_.quota_jobs_per_sec);
        why = msg;
        return false;
    }
    if (b.mpix < mpix) {
        char msg[96];
        std::snprintf(msg, sizeof(msg),
                      "quota: MPix/s budget exhausted (need %.2f, have %.2f)", mpix,
                      b.mpix);
        why = msg;
        return false;
    }
    b.jobs -= 1.0;
    b.mpix -= mpix;
    return true;
}

void DspService::quota_forget(int owner_fd) {
    std::lock_guard<std::mutex> lk(quota_mu_);
    quotas_.erase(owner_fd);
}

DspJobResult DspService::submit_job(const DspJobDesc& desc) {
    DspJobResult res;
    if (!running_.load() || !dsp_ctx_) {
        res.rc = DSP_SVC_ERR_UNAVAILABLE;
        res.message = "DspService not running";
        return res;
    }

    JobRef job;
    std::string why;
    const int vrc = validate_and_pin(desc, job, why);
    if (vrc != DSP_SVC_OK) {
        res.rc = vrc;
        res.message = why;
        {
            std::lock_guard<std::mutex> lk(stats_mu_);
            stats_.jobs_rejected++;
        }
        return res;
    }

    std::string quota_why;
    if (!quota_try_consume(job->owner_fd, job->charge_mpix, quota_why)) {
        unpin_entries(job->pinned);
        res.rc = DSP_SVC_ERR_QUOTA;
        res.message = quota_why;
        {
            std::lock_guard<std::mutex> lk(stats_mu_);
            stats_.jobs_rejected++;
        }
        return res;
    }

    const auto priority = job->priority;
    {
        std::lock_guard<std::mutex> lk(q_mu_);
        (priority == DspPriority::Background ? q_background_ : q_normal_)
            .push_back(job);
    }
    q_cv_.notify_one();

    const auto deadline = std::chrono::steady_clock::now() +
                          std::chrono::milliseconds(cfg_.job_timeout_ms);
    bool completed;
    {
        std::unique_lock<std::mutex> lk(done_mu_);
        completed = done_cv_.wait_until(lk, deadline, [&] { return job->done; });
        if (!completed && !job->done) {
            // Watchdog: a vendor op in flight cannot be cancelled — the
            // worker finishes it and discards the result (jobs_timed_out).
            job->abandoned = true;
        }
    }
    if (job->done) return job->result;

    res.rc = DSP_SVC_ERR_TIMEOUT;
    char msg[128];
    std::snprintf(msg, sizeof(msg), "job timed out after %ums (dst undefined)",
                  cfg_.job_timeout_ms);
    res.message = msg;
    {
        std::lock_guard<std::mutex> lk(stats_mu_);
        stats_.jobs_timed_out++;
    }
    HAL_LOG_WARNING("DspService: %s", msg);
    return res;
}

/* ------------------------------------------------------------------ */
/* Worker                                                              */
/* ------------------------------------------------------------------ */

void DspService::worker_loop() {
    while (running_.load()) {
        JobRef job;
        {
            std::unique_lock<std::mutex> lk(q_mu_);
            q_cv_.wait(lk, [&] {
                return !q_normal_.empty() || !q_background_.empty() ||
                       !running_.load();
            });
            if (!running_.load()) break;
            if (!q_normal_.empty()) {
                job = q_normal_.front();
                q_normal_.pop_front();
            } else {
                job = q_background_.front();
                q_background_.pop_front();
            }
        }
        execute_job(job);
        {
            std::lock_guard<std::mutex> lk(done_mu_);
            job->done = true;
        }
        done_cv_.notify_all();
    }
}

int DspService::build_resize(const JobRef& job, HalDspResizeParams& p) {
    p.src = job->pinned[0]->fb;
    p.dst = job->pinned[1]->fb;
    p.interpolation = job->desc.interpolation;
    return DSP_SVC_OK;
}

int DspService::build_crop_resize(const JobRef& job, HalDspCropResizeParams& p) {
    const DspRect& r = job->desc.rects[0];
    p.src = job->pinned[0]->fb;
    p.dst = job->pinned[1]->fb;
    p.crop.start_x = r.x;
    p.crop.start_y = r.y;
    p.crop.end_x = r.x + r.width;
    p.crop.end_y = r.y + r.height;
    p.interpolation = job->desc.interpolation;
    p.scaling_mode = job->desc.scaling_mode;
    p.letterbox_alignment =
        (job->desc.scaling_mode == HAL_DSP_SCALING_LETTERBOX_MIDDLE)
            ? HAL_DSP_LETTERBOX_MIDDLE
            : (job->desc.scaling_mode == HAL_DSP_SCALING_LETTERBOX_UP_LEFT)
                  ? HAL_DSP_LETTERBOX_UP_LEFT
                  : HAL_DSP_LETTERBOX_NONE;
    p.letterbox_color = HalDspColor{}; /* black */
    return DSP_SVC_OK;
}

int DspService::build_multi_crop(const JobRef& job,
                                 std::vector<HalDspMultiCropOutput>& outputs,
                                 HalDspMultiCropResizeParams& p) {
    p.src = job->pinned[0]->fb;
    p.interpolation = job->desc.interpolation;
    outputs.resize(job->desc.rects.size());
    for (size_t i = 0; i < job->desc.rects.size(); ++i) {
        const DspRect& r = job->desc.rects[i];
        outputs[i].crop.start_x = r.x;
        outputs[i].crop.start_y = r.y;
        outputs[i].crop.end_x = r.x + r.width;
        outputs[i].crop.end_y = r.y + r.height;
        outputs[i].dst = job->pinned[1 + i]->fb;
        outputs[i].scaling_mode = job->desc.scaling_mode;
        outputs[i].letterbox_color = HalDspColor{}; /* black */
    }
    p.outputs = outputs.data();
    p.output_count = static_cast<uint32_t>(outputs.size());
    return DSP_SVC_OK;
}

int DspService::build_convert(const JobRef& job, HalDspConvertFormatParams& p) {
    p.src = job->pinned[0]->fb;
    p.dst = job->pinned[1]->fb;
    return DSP_SVC_OK;
}

void DspService::execute_job(const JobRef& job) {
    const auto t0 = std::chrono::steady_clock::now();
    int rc = DSP_SVC_ERR_INVALID;
    const char* what = "unhandled op";

    HalDspResizeParams rp{};
    HalDspCropResizeParams crp{};
    HalDspConvertFormatParams cfp{};
    std::vector<HalDspMultiCropOutput> outs;
    HalDspMultiCropResizeParams mcp{};

    switch (job->desc.op) {
    case HAL_DSP_OP_RESIZE:
        if (build_resize(job, rp) == DSP_SVC_OK) {
            what = "resize";
            rc = dsp_ops_->resize(dsp_ctx_, &rp);
        }
        break;
    case HAL_DSP_OP_CROP_RESIZE:
        if (build_crop_resize(job, crp) == DSP_SVC_OK) {
            what = "crop_and_resize";
            rc = dsp_ops_->crop_and_resize(dsp_ctx_, &crp);
        }
        break;
    case HAL_DSP_OP_MULTI_CROP_RESIZE:
        if (build_multi_crop(job, outs, mcp) == DSP_SVC_OK) {
            what = "multi_crop_and_resize";
            rc = dsp_ops_->multi_crop_and_resize(dsp_ctx_, &mcp);
        }
        break;
    case HAL_DSP_OP_CONVERT_FORMAT:
        if (build_convert(job, cfp) == DSP_SVC_OK) {
            what = "convert_format";
            rc = dsp_ops_->convert_format(dsp_ctx_, &cfp);
        }
        break;
    default:
        break; /* BLEND etc. land in P1 */
    }

    const auto t1 = std::chrono::steady_clock::now();
    job->result.elapsed_ms = static_cast<uint32_t>(
        std::chrono::duration_cast<std::chrono::milliseconds>(t1 - t0).count());

    if (!job->abandoned) {
        // job->abandoned is written under done_mu_; this read races benignly
        // (worst case a discarded result is also counted as failed).
        if (rc == 0) {
            job->result.rc = DSP_SVC_OK;
            job->result.message = what;
            std::lock_guard<std::mutex> lk(stats_mu_);
            stats_.jobs_ok++;
        } else {
            job->result.rc = rc; /* pass the HAL error through */
            char msg[128];
            std::snprintf(msg, sizeof(msg), "%s failed (HAL rc=%d)", what, rc);
            job->result.message = msg;
            std::lock_guard<std::mutex> lk(stats_mu_);
            stats_.jobs_failed++;
            HAL_LOG_WARNING("DspService: %s", msg);
        }
    }
    unpin_entries(job->pinned);
    job->pinned.clear();
}

DspServiceStats DspService::stats() const {
    std::lock_guard<std::mutex> lk(stats_mu_);
    return stats_;
}
