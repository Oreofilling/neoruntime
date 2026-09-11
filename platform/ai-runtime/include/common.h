#pragma once

#include <cstdint>
#include <string>
#include <functional>
#include <chrono>
#include <sstream>

#include "fd_receiver.h"

extern "C" {
#include "hal_inference.h"
#include "hal_postprocess.h"
}

namespace aipc::ai_runtime {

using SteadyClock  = std::chrono::steady_clock;
using TimePoint    = SteadyClock::time_point;
using Microseconds = std::chrono::microseconds;
using Milliseconds = std::chrono::milliseconds;

inline uint64_t now_us() {
    return std::chrono::duration_cast<Microseconds>(
        SteadyClock::now().time_since_epoch()).count();
}

inline uint64_t now_ns() {
    auto tp = std::chrono::system_clock::now();
    return std::chrono::duration_cast<std::chrono::nanoseconds>(
        tp.time_since_epoch()).count();
}

/// Serialize HalPostprocessResult (C struct) to JSON string for Event Bus publishing.
std::string post_result_to_json(const std::string& stream_id,
                                const std::string& model_id,
                                uint64_t frame_seq,
                                uint64_t timestamp_ns,
                                const HalPostprocessResult& result);

}  // namespace aipc::ai_runtime

// Forward-declare protobuf type to avoid pulling inference.pb.h into common.h.
namespace aipc::inference { class PostResult; }

namespace aipc::ai_runtime {

/// Serialize protobuf PostResult to JSON string.
/// Uses the same field names as post_result_to_json so consumers see a uniform schema.
std::string post_result_pb_to_json(const std::string& stream_id,
                                   const std::string& model_id,
                                   uint64_t frame_seq,
                                   uint64_t timestamp_ns,
                                   const aipc::inference::PostResult& result);

}  // namespace aipc::ai_runtime
