package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"aipc/platform/platform-api/model"
)

// Detection-model postprocess registration guards.
//
// The load-time half of this plumbing — materializing detection models under
// a plugin-recognized basename and composing the variant JSON — lives in
// platform/modelload, shared by platform-api's LoadModel and app-manager's
// PreloadModels so both register models identically. What stays here are the
// checks applied when a model row is written, before anything loads.

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
		fns := make([]string, 0, len(model.DetectionPostprocessProfiles))
		for _, p := range model.DetectionPostprocessProfiles {
			fns = append(fns, p.BackendFunction)
		}
		return fmt.Errorf("variant backend_function %q is not supported — supported functions: %s",
			fn, strings.Join(fns, ", "))
	}
	return nil
}

// validateOutputModeForModel is the server-side guard behind the wizard's
// disabled radio card: the platform postprocess path only decodes NMS-layer
// HEFs, so a feature-map HEF registered as platform+detection would load and
// then fail postprocess on every frame. Raw mode is always fine — the
// consumer owns decoding. Empty vstream info (legacy rows, model_path
// registrations without a prior parse) skips the cross-check rather than
// failing closed on metadata the request never carried.
func validateOutputModeForModel(outputMode, modelType, vstreamInfo string) error {
	if outputMode != model.OutputModePlatform {
		return nil
	}
	if model.ResolveModelType(modelType) != "detection" {
		return nil
	}
	if model.ClassifyOutputFormat(vstreamInfo) != model.OutputFormatFeatureMap {
		return nil
	}
	return fmt.Errorf("this HEF outputs raw feature maps (no NMS layer compiled in), so the platform postprocess cannot decode it — choose raw output mode")
}
