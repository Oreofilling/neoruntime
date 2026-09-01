package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	inferencepb "aipc/platform/ai-runtime/proto"
	"aipc/platform/common/constants"
	"aipc/platform/common/logger"
	"aipc/platform/platform-api/model"
)

// Detection-model postprocess plumbing.
//
// The vendor postprocess plugin (libyolo_hailortpp_post.so, unmodifiable)
// selects its postprocess function from the HEF file basename: HAL rewrites
// the NMS tensor name to <basename>/yolov8_nms_postprocess and the plugin's
// compiled-in basename table never matches the sha256 blob name wizard
// imports get. The helpers below re-materialize such models under a
// recognized basename at load time and compose a schema-valid variant JSON,
// so the stored threshold / max_detections actually reach the runtime.

// modelIDPattern guards the directory name used under <ModelsPath>/runtime/.
// RegisterModel does not sanitize model_id (only empty/duplicate checks) and
// the id becomes a path segment here, so anything outside this set is rejected.
var modelIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// detectionPostprocessProfile returns the stored postprocess_profile for a
// detection model, falling back to the default when Config is missing,
// unparseable, or holds an unknown profile.
func detectionPostprocessProfile(m *model.AIModel) string {
	if m.Config != "" {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(m.Config), &cfg); err == nil {
			if v, ok := cfg["postprocess_profile"].(string); ok {
				if _, valid := model.LookupDetectionProfile(v); valid {
					return v
				}
			}
		}
	}
	return model.DefaultDetectionProfile
}

// runtimeRegistration returns the model path and variant to hand to
// ai-runtime when loading. Non-detection models pass through unchanged.
func runtimeRegistration(m *model.AIModel) (path string, variant string, err error) {
	if model.ResolveModelType(m.ModelType) != "detection" {
		return m.FilePath, m.Variant, nil
	}
	path, err = detectionRuntimePath(m)
	if err != nil {
		return "", "", err
	}
	return path, detectionVariantJSON(m), nil
}

// detectionRuntimePath materializes a detection model under a HEF basename the
// postprocess plugin recognizes: <ModelsPath>/runtime/<model_id>/<profile>.hef
// (hardlink to the stored file, copy fallback for foreign filesystems). Files
// already carrying a recognized basename — built-in disk models — pass through
// unchanged. The DB row is never modified; re-materialization is idempotent.
func detectionRuntimePath(m *model.AIModel) (string, error) {
	base := strings.TrimSuffix(filepath.Base(m.FilePath), filepath.Ext(m.FilePath))
	if _, ok := model.LookupDetectionProfile(base); ok {
		return m.FilePath, nil
	}
	if !modelIDPattern.MatchString(m.ModelID) {
		return "", fmt.Errorf("model id %q cannot be used as a runtime directory name (allowed: letters, digits, '.', '_', '-')", m.ModelID)
	}
	profile := detectionPostprocessProfile(m)
	src := m.FilePath
	if abs, err := filepath.Abs(src); err == nil {
		src = abs
	}
	dir := filepath.Join(constants.ModelsPath(), "runtime", m.ModelID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create runtime model dir: %w", err)
	}
	dst := filepath.Join(dir, profile+".hef")
	// Drop any stale copy from a previous load (older blob or profile).
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to replace stale runtime copy: %w", err)
	}
	if err := os.Link(src, dst); err != nil {
		if err := copyFile(src, dst); err != nil {
			return "", fmt.Errorf("failed to materialize model under postprocess profile: %w", err)
		}
	}
	return dst, nil
}

// detectionVariantJSON builds the model_variant sent to ai-runtime for
// detection models. A JSON-object variant is passed through verbatim (advanced
// escape hatch — its backend_function must match the selected profile);
// anything else is replaced with a schema-valid blob composed from the
// selected postprocess profile plus the stored threshold and max detections,
// which the plugin reads as detection_threshold / max_boxes. Labels are
// deliberately absent: the plugin's label table is compiled in and ignores
// JSON labels; consumers map class_id N -> labels[N-1] from stored metadata.
func detectionVariantJSON(m *model.AIModel) string {
	if model.ResolveModelType(m.ModelType) != "detection" {
		return m.Variant
	}
	if strings.HasPrefix(strings.TrimSpace(m.Variant), "{") {
		return m.Variant
	}
	profile, _ := model.LookupDetectionProfile(detectionPostprocessProfile(m))
	cfg := map[string]interface{}{
		"backend_function": profile.BackendFunction,
	}
	if m.Threshold > 0 {
		cfg["detection_threshold"] = m.Threshold
	}
	if m.MaxDetections > 0 {
		cfg["max_boxes"] = m.MaxDetections
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		// Unreachable for these value types; keep the stored variant if so.
		return m.Variant
	}
	return string(blob)
}

// removeRuntimeCopy deletes the materialized runtime copy of a model, if any.
// The CAS blob is not touched (its refcount is handled by the caller).
func removeRuntimeCopy(modelID string) {
	if !modelIDPattern.MatchString(modelID) {
		return
	}
	if err := os.RemoveAll(filepath.Join(constants.ModelsPath(), "runtime", modelID)); err != nil {
		logger.Warn("Failed to remove runtime copy for model %s: %v", modelID, err)
	}
}

// runLoadSmokeTest probes a freshly loaded model with one zero-filled input
// tensor. Detection postprocess failures (NMS tensor-name mismatches in the
// vendor plugin) only surface at infer time — RegisterModel succeeds and
// every frame then fails — so a probe infer at load time catches them while
// the caller can still roll the registration back. A missing modelInfo skips
// the probe (nothing to size a tensor from).
func runLoadSmokeTest(ctx context.Context, client inferencepb.InferenceServiceClient, modelID string, modelInfo *inferencepb.ModelInfo) error {
	if modelInfo == nil || len(modelInfo.Inputs) == 0 {
		logger.Warn("Skipping load smoke test for %s: no input tensor info available", modelID)
		return nil
	}
	tensor, err := zeroInputTensor(modelInfo.Inputs[0])
	if err != nil {
		return fmt.Errorf("cannot build probe tensor: %w", err)
	}
	smokeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := client.Infer(smokeCtx, &inferencepb.InferRequest{
		ModelId:   modelID,
		Inputs:    []*inferencepb.Tensor{tensor},
		TimeoutMs: 10000,
	})
	if err != nil {
		return fmt.Errorf("probe infer failed: %w", err)
	}
	if resp.GetStatus() != nil && !resp.GetStatus().GetSuccess() {
		return fmt.Errorf("%s", resp.GetStatus().GetMessage())
	}
	return nil
}

func dtypeSize(dt inferencepb.DataType) int {
	switch dt {
	case inferencepb.DataType_UINT8, inferencepb.DataType_INT8:
		return 1
	case inferencepb.DataType_UINT16, inferencepb.DataType_INT16, inferencepb.DataType_FLOAT16:
		return 2
	case inferencepb.DataType_FLOAT32, inferencepb.DataType_INT32, inferencepb.DataType_UINT32:
		return 4
	default:
		return 0
	}
}

// zeroInputTensor mirrors an input TensorSpec into a zero-filled Tensor sized
// from the spec's own shape and dtype (input layouts vary — NV12 inputs are
// H*W*1.5 bytes — so the size is derived, never assumed).
func zeroInputTensor(spec *inferencepb.TensorSpec) (*inferencepb.Tensor, error) {
	elem := dtypeSize(spec.GetDtype())
	if elem == 0 {
		return nil, fmt.Errorf("unsupported input dtype %v", spec.GetDtype())
	}
	total := elem
	for _, d := range spec.GetShape() {
		if d <= 0 {
			return nil, fmt.Errorf("invalid input shape %v", spec.GetShape())
		}
		total *= int(d)
	}
	return &inferencepb.Tensor{
		Shape: spec.GetShape(),
		Dtype: spec.GetDtype(),
		Data:  make([]byte, total),
	}, nil
}
