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

// Tuning defaults for rows stored before the columns existed (or zeroed by
// hand): they mirror the gorm column defaults on AIModel and the NMS default
// from the reference config the vendor plugin ships.
const (
	defaultDetectionThreshold = 0.25
	defaultNmsThreshold       = 0.45
	defaultMaxDetections      = 64
)

// defaultPostprocessLabels mirrors the plugin's compiled-in label table
// shape: index 0 is a placeholder so labels[N] names class_id N.
var defaultPostprocessLabels = []string{"unlabeled", "person", "vehicle", "face", "license_plate"}

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

// customVariantKeys are the keys the vendor plugin's JSON schema requires —
// a partial blob is rejected at load time with a bare "required" error, so
// user-supplied variant JSON must carry all of them up front.
var customVariantKeys = []string{
	"backend_function",
	"iou_threshold",
	"detection_threshold",
	"output_activation",
	"label_offset",
	"max_boxes",
	"labels",
}

// validateDetectionVariant guards the advanced `{…}` variant escape hatch
// before it is stored: every schema key must be present and backend_function
// must name one of the verified postprocess functions (the generic
// single-argument ones hardcode a 0.4 threshold and COCO labels, so a model
// pointed at them would silently detect with the wrong settings). Variants
// that are not JSON objects — plugin basenames, keypoint names, empty — are
// left to their existing handling and pass unchanged.
func validateDetectionVariant(variant string) error {
	trimmed := strings.TrimSpace(variant)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return fmt.Errorf("variant is not valid JSON: %w", err)
	}
	var missing []string
	for _, key := range customVariantKeys {
		if _, ok := cfg[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("variant JSON is missing required key(s): %s — the postprocess plugin requires all of: %s",
			strings.Join(missing, ", "), strings.Join(customVariantKeys, ", "))
	}
	fn, _ := cfg["backend_function"].(string)
	if _, ok := model.LookupDetectionBackendFunction(fn); !ok {
		return fmt.Errorf("variant backend_function %q is not supported — supported functions: hailo_yolov8n, hailo_yolov8s, hailo_yolov8m",
			fn)
	}
	return nil
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
// anything else is replaced with the plugin's full config blob, composed from
// the selected postprocess profile plus stored tuning values. The vendor
// postprocess plugin rejects partial JSON against its schema ("required"
// fields), so every key must be present — device-verified 2026-09-02. Of
// those keys the hailo_* functions only read detection_threshold and
// max_boxes; labels come from a compiled-in table, so the JSON labels
// (config labels with a leading placeholder, index 0) are advisory only —
// consumers map class_id N -> labels[N-1] from stored metadata.
func detectionVariantJSON(m *model.AIModel) string {
	if model.ResolveModelType(m.ModelType) != "detection" {
		return m.Variant
	}
	if strings.HasPrefix(strings.TrimSpace(m.Variant), "{") {
		return m.Variant
	}
	profile, _ := model.LookupDetectionProfile(detectionPostprocessProfile(m))
	threshold := m.Threshold
	if threshold <= 0 {
		threshold = defaultDetectionThreshold
	}
	maxBoxes := m.MaxDetections
	if maxBoxes <= 0 {
		maxBoxes = defaultMaxDetections
	}
	cfg := map[string]interface{}{
		"backend_function":    profile.BackendFunction,
		"iou_threshold":       detectionNmsThreshold(m),
		"detection_threshold": threshold,
		"output_activation":   "none",
		"label_offset":        1,
		"max_boxes":           maxBoxes,
		"labels":              detectionLabels(m),
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		// Unreachable for these value types; keep the stored variant if so.
		return m.Variant
	}
	return string(blob)
}

// detectionNmsThreshold reads nms_threshold from the schema-driven Config
// JSON, falling back to the schema default for rows without one.
func detectionNmsThreshold(m *model.AIModel) float64 {
	if m.Config != "" {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(m.Config), &cfg); err == nil {
			if v, ok := cfg["nms_threshold"].(float64); ok && v > 0 {
				return v
			}
		}
	}
	return defaultNmsThreshold
}

// detectionLabels turns the comma-separated labels config into the JSON
// labels array the plugin schema requires, keeping index 0 a placeholder so
// labels[N] names class_id N. Without stored labels the reference default
// table is used.
func detectionLabels(m *model.AIModel) []string {
	if m.Config != "" {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(m.Config), &cfg); err == nil {
			if raw, ok := cfg["labels"].(string); ok && strings.TrimSpace(raw) != "" {
				parts := strings.Split(raw, ",")
				labels := []string{"unlabeled"}
				for _, p := range parts {
					if name := strings.TrimSpace(p); name != "" {
						labels = append(labels, name)
					}
				}
				if len(labels) > 1 {
					return labels
				}
			}
		}
	}
	return defaultPostprocessLabels
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

// zeroInputTensor mirrors an input TensorSpec into a zero-filled Tensor. The
// size comes from the spec's byte_size when the runtime provides it — NV12
// inputs report an RGB-like shape whose product is not the host frame size
// (W*H*3/2 is) — falling back to shape × dtype for specs without one.
func zeroInputTensor(spec *inferencepb.TensorSpec) (*inferencepb.Tensor, error) {
	total := int(spec.GetByteSize())
	if total == 0 {
		elem := dtypeSize(spec.GetDtype())
		if elem == 0 {
			return nil, fmt.Errorf("unsupported input dtype %v", spec.GetDtype())
		}
		total = elem
		for _, d := range spec.GetShape() {
			if d <= 0 {
				return nil, fmt.Errorf("invalid input shape %v", spec.GetShape())
			}
			total *= int(d)
		}
	}
	return &inferencepb.Tensor{
		Shape: spec.GetShape(),
		Dtype: spec.GetDtype(),
		Data:  make([]byte, total),
	}, nil
}
