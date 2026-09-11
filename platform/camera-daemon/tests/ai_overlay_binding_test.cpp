/**
 * @file ai_overlay_binding_test.cpp
 * @brief Behavior decoupling + frame-sync hardening — the scope-cut of
 *        the video-fd-output-isolation draft (D output-buffer chain
 *        deferred; see plan "输出隔离裁剪版").
 *
 * Contract under test (new defaults, legacy opt-in):
 *
 *   Admission — platform events (source "ai-runtime"/"auto-infer" or the
 *   empty default of the 3-arg test call) are stored ONLY when
 *   legacy_auto_bind is on or the stream is in overlay.bindings; anything
 *   else is dropped and counted (overlay_no_binding_drops). App events
 *   (any other source, i.e. an SDK app_id) are ALWAYS stored — a bare
 *   subscribe() must not draw, and an app's own annotate results must
 *   never be crowded out by admission.
 *
 *   Layers — results are no longer a single slot per stream: each
 *   (session_id, source_id) identity is a layer. Same identity replaces;
 *   different identities stack. `{"polygons":[]}` clears only ITS layer's
 *   polygons (P2-11 clear-stale-boxes semantics preserved per layer).
 *
 *   Suppression — a fresh (TTL-valid) app layer suppresses platform
 *   layers on the same stream at draw time; platform layers stack with
 *   each other. Draw order is fixed: app layers (newest first), then
 *   platform layers (storage order).
 *
 *   Session end — rule 1: polygons of any surviving layer whose polygon
 *   write was tagged with the session are cleared; rule 2: app layers
 *   created by that session are erased outright. Each layer counts once.
 *
 *   Epoch / frame binding — app events may carry metadata stream_epoch /
 *   frame_sequence. A stale epoch is rejected (overlay_epoch_rejects); a
 *   frame_sequence far past the last frame the bake site saw is judged
 *   from a dead generation and rejected (overlay_late_commands). A bound
 *   layer draws only while current_seq <= bound_seq + kFrameBindSlack(2).
 *   note_stream_restart() bumps the epoch and purges the stream's layers.
 *
 * Host-only harness: plain main() + assert(), same style as the session
 * test. Draw observability = a zeroed HalDrawOps with ONLY draw_result
 * set to a call counter (every drawn layer makes exactly one call).
 */

#include <cassert>
#include <cstdio>
#include <map>
#include <string>

#include <unistd.h> /* usleep */

#include "ai_overlay_subscriber.h"

/* ---------------- shared payloads / helpers ---------------- */

static const char* kDetPayload =
    "{\"num_detections\":1,\"detections\":[{\"bbox\":[0.1,0.1,0.2,0.2],"
    "\"score\":0.9,\"label\":\"person\"}]}";

static const char* kZonePayload =
    "{\"polygons\":[{\"points\":[[0.1,0.1],[0.9,0.1],[0.9,0.9]],"
    "\"label\":\"zone\"}]}";

static const char* kZone2Payload =
    "{\"polygons\":[{\"points\":[[0.1,0.1],[0.5,0.1],[0.5,0.5]],"
    "\"label\":\"a\"},{\"points\":[[0.2,0.2],[0.8,0.2],[0.8,0.8]],"
    "\"label\":\"b\"}]}";

static const char* kEmptyPolysPayload = "{\"polygons\":[]}";

/* auto-infer events carry NO metadata — stream_id rides in the payload */
static const char* kDetWithStreamId =
    "{\"stream_id\":\"main\",\"num_detections\":1,\"detections\":["
    "{\"bbox\":[0.1,0.1,0.2,0.2],\"score\":0.9,\"label\":\"car\"}]}";

static HalFrameBuffer make_frame() {
    HalFrameBuffer fb{};
    fb.width = 64;
    fb.height = 48;
    fb.format = HAL_PIX_FMT_NV12;
    fb.mem_type = HAL_MEM_DMABUF;
    fb.num_planes = 2;
    for (uint32_t p = 0; p < HAL_MAX_PLANES; ++p) fb.dma_fds[p] = -1;
    fb.strides[0] = 64;
    fb.sizes[0] = 64 * 48;
    fb.strides[1] = 64;
    fb.sizes[1] = 64 * 48 / 2;
    return fb;
}

static int g_draw_calls = 0;
static int counting_draw_result(const HalPostprocessResult*, HalFrameBuffer*,
                                const HalDrawConfig*) {
    ++g_draw_calls;
    return 0;
}

/* Identity-fed, enabled, counting draw op — the minimum config that
 * reaches the draw path for stream "main". */
static AiOverlayConfig base_cfg() {
    AiOverlayConfig cfg;
    cfg.enabled = true;
    cfg.stream_map["main"] = "main";
    static HalDrawOps ops = [] {
        HalDrawOps o = {};
        o.draw_result = counting_draw_result;
        return o;
    }();
    cfg.draw_ops = &ops;
    return cfg;
}

static AiOverlaySubscriber::OverlayStreamStats stats_of(
    AiOverlaySubscriber& sub, const char* stream) {
    AiOverlaySubscriber::OverlayStreamStats os{};
    sub.snapshot_stream_stats(stream, &os);
    return os;
}

static std::map<std::string, std::string> md_stream() {
    return {{"stream_id", "main"}};
}

/* ---------------- admission ---------------- */

static void case_platform_dropped_by_default() {
    /* New contract default: no legacy switch, no explicit binding → a
     * platform result event (3-arg call = empty source = platform) is
     * dropped on the floor and COUNTED. A bare subscribe() must not
     * change the video. */
    AiOverlaySubscriber sub(AiOverlayConfig{});
    sub.handle_event("inference/main", md_stream(), kZonePayload);
    const auto os = stats_of(sub, "main");
    assert(os.overlay_layer_count == 0);
    assert(os.overlay_no_binding_drops == 1);
    assert(sub.polygon_count("main") == 0);
    std::printf("  case_platform_dropped_by_default ok\n");
}

static void case_legacy_auto_bind_admits() {
    /* legacy_auto_bind keeps today's behavior: everything platform is
     * admitted, no drops. */
    AiOverlayConfig cfg;
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);
    sub.handle_event("inference/main", md_stream(), kZonePayload);
    const auto os = stats_of(sub, "main");
    assert(os.overlay_layer_count == 1);
    assert(os.overlay_no_binding_drops == 0);
    assert(sub.polygon_count("main") == 1);
    std::printf("  case_legacy_auto_bind_admits ok\n");
}

static void case_explicit_bindings_admit() {
    /* The forward path: declare infer-stream → display-stream bindings
     * in daemon config; the bound stream's platform results draw. */
    AiOverlayConfig cfg;
    cfg.bindings["main"] = "main";
    AiOverlaySubscriber sub(cfg);
    sub.handle_event("inference/main", md_stream(), kZonePayload);
    const auto os = stats_of(sub, "main");
    assert(os.overlay_layer_count == 1);
    assert(os.overlay_no_binding_drops == 0);
    assert(sub.polygon_count("main") == 1);
    /* an unbound stream is still dropped */
    sub.handle_event("inference/other", {{"stream_id", "other"}},
                     kZonePayload);
    assert(stats_of(sub, "main").overlay_no_binding_drops == 0);
    assert(stats_of(sub, "other").overlay_no_binding_drops == 1);
    std::printf("  case_explicit_bindings_admit ok\n");
}

static void case_autoinfer_payload_stream_fallback() {
    /* auto-infer publishes topic inference/<stream> with NO metadata —
     * today that event is silently dropped because stream_id is only
     * read from metadata. The payload fallback fixes the drop. */
    AiOverlayConfig cfg;
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);
    sub.handle_event("inference/main", {}, kDetWithStreamId, "auto-infer");
    const auto os = stats_of(sub, "main");
    assert(os.overlay_layer_count == 1);
    assert(os.overlay_no_binding_drops == 0);
    std::printf("  case_autoinfer_payload_stream_fallback ok\n");
}

static void case_app_events_always_stored() {
    /* App annotate results bypass admission entirely — the new default
     * (platform dropped) must not starve the app's own channel. */
    AiOverlaySubscriber sub(AiOverlayConfig{});
    sub.handle_event("inference/main", md_stream(), kZonePayload, "app-1");
    const auto os = stats_of(sub, "main");
    assert(os.overlay_layer_count == 1);
    assert(os.overlay_no_binding_drops == 0);
    assert(sub.polygon_count("main") == 1);
    std::printf("  case_app_events_always_stored ok\n");
}

/* ---------------- layers ---------------- */

static void case_layer_replace_and_stack() {
    /* (session "", source) is the layer identity for platform events;
     * (session, app source_id) for app events. Same key replaces the
     * layer wholesale; different keys stack. polygon_count sums layers. */
    AiOverlayConfig cfg;
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);

    /* platform, no model metadata → source_id "" */
    sub.handle_event("inference/main", md_stream(), kZonePayload);
    /* platform, model m1 → source_id "/m1" (prod: "ai-runtime/m1") */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"model_id", "m1"}},
                     kZonePayload);
    /* app → its own layer */
    sub.handle_event("inference/main", md_stream(), kZonePayload, "app-1");

    assert(stats_of(sub, "main").overlay_layer_count == 3);
    assert(sub.polygon_count("main") == 3);

    /* same identities replace, not stack */
    sub.handle_event("inference/main", md_stream(), kZone2Payload, "app-1");
    assert(stats_of(sub, "main").overlay_layer_count == 3);
    assert(sub.polygon_count("main") == 4); /* 1 + 1 + 2 */

    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"model_id", "m1"}},
                     kZone2Payload);
    assert(stats_of(sub, "main").overlay_layer_count == 3);
    assert(sub.polygon_count("main") == 5); /* 1 + 2 + 2 */
    std::printf("  case_layer_replace_and_stack ok\n");
}

/* ---------------- draw: suppression + binding expiry ---------------- */

static void case_app_suppresses_platform_draw() {
    /* Fresh app layer wins the stream: platform layers are suppressed at
     * DRAW time while the app layer is TTL-valid, and come back once it
     * expires. One drawn layer = one draw_result call. */
    AiOverlayConfig cfg = base_cfg();
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);
    HalFrameBuffer fb = make_frame();

    /* platform alone → 1 call */
    sub.handle_event("inference/main", md_stream(), kDetPayload);
    g_draw_calls = 0;
    sub.apply_overlay("main", &fb);
    assert(g_draw_calls == 1);

    /* fresh app layer (ttl 50ms) suppresses the platform layer → still 1 */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"result_ttl_ms", "50"}},
                     kDetPayload, "app-1");
    g_draw_calls = 0;
    sub.apply_overlay("main", &fb);
    assert(g_draw_calls == 1); /* app only, platform suppressed */

    /* app layer expires (50ms + slack) → platform draws again */
    usleep(120 * 1000);
    g_draw_calls = 0;
    sub.apply_overlay("main", &fb);
    assert(g_draw_calls == 1); /* platform only, app expired */
    std::printf("  case_app_suppresses_platform_draw ok\n");
}

static void case_bound_layer_draw_expiry() {
    /* A layer bound to frame_sequence N draws while
     * current_seq <= N + kFrameBindSlack(2), then stops. An unbound
     * layer always draws. */
    AiOverlayConfig cfg = base_cfg();
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);
    HalFrameBuffer fb = make_frame();

    /* platform: unbound; app: bound at 102 */
    sub.handle_event("inference/main", md_stream(), kDetPayload);
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"frame_sequence", "102"}},
                     kDetPayload, "app-1");

    g_draw_calls = 0;
    sub.apply_overlay("main", &fb, 103);
    assert(g_draw_calls == 2); /* 103 <= 102+2: both */
    sub.apply_overlay("main", &fb, 104);
    assert(g_draw_calls == 4); /* 104 <= 104: both */
    sub.apply_overlay("main", &fb, 105);
    assert(g_draw_calls == 5); /* bound expired: platform only */
    sub.apply_overlay("main", &fb, 1000);
    assert(g_draw_calls == 6); /* unbound draws regardless */
    std::printf("  case_bound_layer_draw_expiry ok\n");
}

/* ---------------- [] clear + session end ---------------- */

static void case_empty_polygons_clears_only_that_layer() {
    /* `{"polygons":[]}` clears the polygons of ITS layer only — another
     * layer's zones on the same stream survive. */
    AiOverlayConfig cfg;
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);

    sub.handle_event("inference/main", md_stream(), kZonePayload);
    sub.handle_event("inference/main", md_stream(), kZonePayload, "app-1");
    assert(sub.polygon_count("main") == 2);

    sub.handle_event("inference/main", md_stream(), kEmptyPolysPayload,
                     "app-1");
    assert(sub.polygon_count("main") == 1); /* platform layer keeps its zone */
    assert(stats_of(sub, "main").overlay_layer_count == 2);
    std::printf("  case_empty_polygons_clears_only_that_layer ok\n");
}

static void case_session_end_erases_app_layers() {
    /* Rule 2: app layers created by the session are erased. Rule 1:
     * polygons WRITES tagged with the session are cleared from any
     * surviving layer (the platform layer here). Each layer counts once. */
    AiOverlayConfig cfg;
    cfg.legacy_auto_bind = true;
    AiOverlaySubscriber sub(cfg);

    /* app layer owned by session s1 */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"session_id", "s1"}},
                     kZonePayload, "app-1");
    /* untagged app layer — operator-side write, never swept */
    sub.handle_event("inference/main", md_stream(), kZonePayload, "app-2");
    /* platform layer whose polygon write is tagged s1 (sidecar sweep) */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"session_id", "s1"}},
                     kZonePayload);
    assert(sub.polygon_count("main") == 3);
    assert(stats_of(sub, "main").overlay_layer_count == 3);

    const size_t swept = sub.handle_session_end("s1");
    assert(swept == 2); /* erased app layer + swept platform polygons */
    assert(stats_of(sub, "main").overlay_layer_count == 2);
    assert(sub.polygon_count("main") == 1); /* only untagged app zone */

    assert(sub.handle_session_end("s1") == 0); /* idempotent */
    std::printf("  case_session_end_erases_app_layers ok\n");
}

/* ---------------- epoch + late frame_sequence ---------------- */

static void case_epoch_reject_and_restart_purge() {
    AiOverlaySubscriber sub(AiOverlayConfig{});

    const uint64_t e0 = stats_of(sub, "main").stream_epoch;
    assert(e0 != 0); /* CLOCK_MONOTONIC-us seed */

    /* correct epoch admitted */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"stream_epoch",
                                              std::to_string(e0)}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_layer_count == 1);
    assert(sub.polygon_count("main") == 1);
    assert(stats_of(sub, "main").overlay_epoch_rejects == 0);

    /* stale epoch rejected + counted */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"},
                      {"stream_epoch", std::to_string(e0 + 7)}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_epoch_rejects == 1);
    assert(stats_of(sub, "main").overlay_layer_count == 1);

    /* restart bumps the epoch AND purges the stream's layers */
    sub.note_stream_restart("main");
    assert(stats_of(sub, "main").stream_epoch == e0 + 1);
    assert(stats_of(sub, "main").overlay_layer_count == 0);
    assert(sub.polygon_count("main") == 0);

    /* pre-restart epoch now rejected */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"},
                      {"stream_epoch", std::to_string(e0)}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_epoch_rejects == 2);
    assert(stats_of(sub, "main").overlay_layer_count == 0);

    /* current epoch accepted again */
    sub.handle_event("inference/main",
                     {{"stream_id", "main"},
                      {"stream_epoch", std::to_string(e0 + 1)}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_layer_count == 1);
    std::printf("  case_epoch_reject_and_restart_purge ok\n");
}

static void case_late_frame_sequence_rejected() {
    /* frame_sequence far past the last frame the bake site saw = a
     * command from a dead frame generation → rejected + counted. Within
     * the slack (kFrameBindSlack=2) it is a legitimate in-flight bind. */
    AiOverlaySubscriber sub(AiOverlayConfig{});
    HalFrameBuffer fb = make_frame();

    /* the bake site saw frame 100 (draw_ops null: stored anchor only) */
    sub.apply_overlay("main", &fb, 100);

    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"frame_sequence", "1000"}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_late_commands == 1);
    assert(stats_of(sub, "main").overlay_layer_count == 0);

    sub.handle_event("inference/main",
                     {{"stream_id", "main"}, {"frame_sequence", "102"}},
                     kZonePayload, "app-1");
    assert(stats_of(sub, "main").overlay_late_commands == 1); /* unchanged */
    assert(stats_of(sub, "main").overlay_layer_count == 1);
    std::printf("  case_late_frame_sequence_rejected ok\n");
}

int main() {
    case_platform_dropped_by_default();
    case_legacy_auto_bind_admits();
    case_explicit_bindings_admit();
    case_autoinfer_payload_stream_fallback();
    case_app_events_always_stored();
    case_layer_replace_and_stack();
    case_app_suppresses_platform_draw();
    case_bound_layer_draw_expiry();
    case_empty_polygons_clears_only_that_layer();
    case_session_end_erases_app_layers();
    case_epoch_reject_and_restart_purge();
    case_late_frame_sequence_rejected();
    std::printf("ai_overlay_binding_test: all assertions passed\n");
    return 0;
}
