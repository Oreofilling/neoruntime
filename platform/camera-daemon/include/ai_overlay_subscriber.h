/**
 * @file ai_overlay_subscriber.h
 * @brief AI Overlay Subscriber — receives inference results from Event Bus
 *        and draws AI overlays (detection boxes, landmarks, etc.) on video
 *        frames before encoding.
 *
 * Uses HalDrawOps + HalPostprocessResult.
 */

#pragma once

#include <string>
#include <unordered_map>
#include <mutex>
#include <atomic>
#include <thread>
#include <memory>
#include <chrono>

extern "C" {
    #include "hal_postprocess.h"
    #include "hal_draw.h"
    #include "hal_buffer.h"
}

struct AiOverlayConfig {
    bool        enabled = false;
    std::string event_bus_endpoint = "unix:///run/aipc/event-bus.sock";
    std::string topic_prefix       = "inference/";

    bool     draw_detections  = true;
    bool     draw_labels      = true;
    bool     draw_confidence  = true;
    bool     draw_landmarks   = true;
    bool     enable_face_blur = false;
    uint32_t face_blur_block_size = 8;   // mosaic cell size (px); 0 = blur
    uint32_t box_thickness    = 2;

    std::unordered_map<std::string, std::string> stream_map;

    const HalDrawOps* draw_ops = nullptr;
};

// Collects pixel-space mosaic rects for detections labeled "face"
// (case-insensitive). Normalized bboxes are scaled to the frame and clipped to
// its bounds (hal_bbox_to_rect semantics); degenerate rects are skipped.
// Returns the number of rects written to out, at most cap. Free function so
// the geometry is unit-testable without a live subscriber or frame.
size_t collect_face_mosaic_rects(const HalPostprocessResult& result,
                                 uint32_t frame_width, uint32_t frame_height,
                                 uint32_t block_size,
                                 HalDrawMosaic* out, size_t cap);

class AiOverlaySubscriber {
public:
    explicit AiOverlaySubscriber(const AiOverlayConfig& config);
    ~AiOverlaySubscriber();

    AiOverlaySubscriber(const AiOverlaySubscriber&) = delete;
    AiOverlaySubscriber& operator=(const AiOverlaySubscriber&) = delete;

    bool start();
    void stop();

    void apply_overlay(const std::string& stream_name, HalFrameBuffer* frame);

    bool is_running() const { return running_.load(); }

    void update_config(bool draw_labels, bool draw_confidence, uint32_t box_thickness,
                       bool enable_face_blur);

private:
    struct StreamResult {
        HalPostprocessResult result{};
        HalDrawConfig        draw_cfg{};
        uint64_t          last_frame_seq = 0;
        std::chrono::steady_clock::time_point last_update_time;
        bool              valid = false;
    };

    void subscriber_loop();

    void draw_with_primitives(const HalPostprocessResult& result, HalFrameBuffer* frame,
                              bool draw_labels, bool draw_confidence, uint32_t box_thickness,
                              bool enable_face_blur, uint32_t face_blur_block_size);
    bool parse_json_result(const std::string& payload, const std::string& stream_id,
                           HalPostprocessResult* out);

    AiOverlayConfig config_;

    // Protects mutable config fields (draw_labels, draw_confidence, box_thickness,
    // enable_face_blur)
    mutable std::mutex config_mu_;

    HalDrawConfig default_draw_cfg_{};

    std::atomic<bool> running_{false};
    std::thread       subscriber_thread_;

    mutable std::mutex ctx_mu_;
    void*              active_ctx_ = nullptr;

    mutable std::mutex results_mu_;
    std::unordered_map<std::string, StreamResult> results_;
};
