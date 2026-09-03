// Bundled private-model packages: unpacking the AMPK .bin containers that
// app manifests declare under spec.models.<alias>.path. Install (extractImageModels)
// turns each package into an on-disk HEF plus this sidecar record; PreloadModels
// reads the sidecar back at every reboot to re-register the model with the
// exact same gRPC registration (the .bin itself is deleted after unpack, so
// the registration cannot be re-derived from it).
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aipc/platform/modelload"
	"aipc/platform/platform-api/model"
	"aipc/platform/platform-api/storage"
)

// bundledRegistrationFile is the sidecar written next to the unpacked HEF.
const bundledRegistrationFile = "registration.json"

// bundledRegistration is the durable record of one unpacked bundled model.
// ModelType/ModelVariant are the gRPC registration values modelload composed
// at install time (empty ModelType = raw output, no postprocess session).
type bundledRegistration struct {
	ModelID      string `json:"model_id"`
	HEF          string `json:"hef"` // basename, inside app-models/<app>/<alias>/
	ModelType    string `json:"model_type"`
	ModelVariant string `json:"model_variant,omitempty"`
}

// unpackBundledPackage opens the AMPK package at binPath, verifies it, stages
// the embedded HEF into aliasDir and composes the gRPC registration the same
// way platform-api's RegisterModel API does (schema defaults merged with the
// package config, threshold/max_detections lifted into the columns the
// variant composer reads). Detection models in platform mode are staged under
// their postprocess profile basename, so modelload.RuntimeRegistration passes
// the file through unchanged — no copy under /data models runtime/.
func unpackBundledPackage(binPath, aliasDir, modelID string) (*bundledRegistration, error) {
	f, err := os.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundled package: %w", err)
	}
	defer f.Close()
	pr, err := storage.OpenPackage(f)
	if err != nil {
		return nil, err
	}
	meta := pr.Meta()

	modelType := model.ResolveModelType(meta.ModelType)
	if modelType == "" {
		return nil, fmt.Errorf("bundled package declares unknown model_type %q", meta.ModelType)
	}
	outputMode, ok := model.ResolveOutputMode(meta.OutputMode)
	if !ok {
		return nil, fmt.Errorf("bundled package declares unsupported output_mode %q", meta.OutputMode)
	}

	merged := model.GetFieldDefaults(modelType)
	if merged == nil {
		merged = make(map[string]interface{})
	}
	if len(meta.Config) > 0 {
		var pkgCfg map[string]interface{}
		if err := json.Unmarshal(meta.Config, &pkgCfg); err != nil {
			return nil, fmt.Errorf("bundled package config is not a JSON object: %w", err)
		}
		for k, v := range pkgCfg {
			merged[k] = v
		}
	}
	configJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode merged config: %w", err)
	}

	synth := &model.AIModel{
		ModelID:    modelID,
		FilePath:   "", // filled in once the destination basename is known
		ModelType:  modelType,
		OutputMode: outputMode,
		Config:     string(configJSON),
	}
	// Column lift, mirroring RegisterModel: DetectionVariantJSON composes from
	// the Threshold/MaxDetections columns, not from Config.
	if v, ok := merged["threshold"].(float64); ok {
		synth.Threshold = float32(v)
	}
	if v, ok := merged["max_detections"].(float64); ok {
		synth.MaxDetections = int(v)
	}

	base, err := bundledHEFBasename(synth, meta, modelID)
	if err != nil {
		return nil, fmt.Errorf("bundled package postprocess_profile is not usable: %w", err)
	}
	synth.FilePath = filepath.Join(aliasDir, base)

	tmp := synth.FilePath + ".tmp"
	if err := writeVerifiedHEF(tmp, pr); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, synth.FilePath); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("failed to publish bundled HEF: %w", err)
	}

	hefPath, variant, grpcType, err := modelload.RuntimeRegistration(synth)
	if err != nil {
		return nil, fmt.Errorf("failed to compose runtime registration: %w", err)
	}

	reg := &bundledRegistration{
		ModelID:      modelID,
		HEF:          filepath.Base(hefPath),
		ModelType:    grpcType,
		ModelVariant: variant,
	}
	blob, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode registration record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, bundledRegistrationFile), blob, 0o644); err != nil {
		return nil, fmt.Errorf("failed to persist registration record: %w", err)
	}
	return reg, nil
}

// bundledHEFBasename picks the on-disk name for the unpacked HEF. Detection
// models in platform mode take their postprocess profile basename (the
// plugin-basename passthrough in modelload); everything else keeps the
// package's original filename when it is a usable .hef name. A package whose
// config names an unknown postprocess_profile is an error — installing it
// would silently mismatch the model to the default profile later.
func bundledHEFBasename(synth *model.AIModel, meta *storage.PackageMeta, modelID string) (string, error) {
	if synth.ModelType == "detection" && synth.OutputMode == model.OutputModePlatform {
		profile, err := modelload.DetectionPostprocessProfile(synth)
		if err != nil {
			return "", err
		}
		return profile + ".hef", nil
	}
	dest := filepath.Base(meta.HEF.Filename)
	if filepath.Ext(dest) != ".hef" {
		dest = safeHEFBasename(modelID)
	}
	return dest, nil
}

// safeHEFBasename derives a fallback filename from the model id; ids that
// cannot serve as a filename (empty, dot-only, path separators) degrade to a
// constant so the name always stays a single path segment.
func safeHEFBasename(modelID string) string {
	if modelID != "" && modelID != "." && modelID != ".." && !strings.ContainsAny(modelID, `/\`) {
		return modelID + ".hef"
	}
	return "bundled.hef"
}

// writeVerifiedHEF streams the package's HEF section into path and verifies
// the package digest once every byte is staged. path is a staging name: the
// caller publishes it by rename and removes it on failure, so the bytes never
// sit at their final path before Verify passes.
func writeVerifiedHEF(path string, pr *storage.PackageReader) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to stage bundled HEF: %w", err)
	}
	if _, err := io.Copy(out, pr.HEF()); err != nil {
		out.Close()
		return fmt.Errorf("failed to unpack bundled HEF: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close staged HEF: %w", err)
	}
	if err := pr.Verify(); err != nil {
		return err
	}
	return nil
}

// loadBundledRegistration reads the sidecar unpackBundledPackage wrote. It is
// the authoritative registration record at reboot: the .bin was deleted after
// unpack, so the composed type/variant cannot be re-derived.
func loadBundledRegistration(aliasDir string) (*bundledRegistration, error) {
	blob, err := os.ReadFile(filepath.Join(aliasDir, bundledRegistrationFile))
	if err != nil {
		return nil, fmt.Errorf("bundled registration record missing: %w", err)
	}
	reg := &bundledRegistration{}
	if err := json.Unmarshal(blob, reg); err != nil {
		return nil, fmt.Errorf("bundled registration record corrupt: %w", err)
	}
	if reg.ModelID == "" || reg.HEF == "" || reg.HEF == "." || reg.HEF == ".." || strings.ContainsAny(reg.HEF, `/\`) {
		return nil, fmt.Errorf("bundled registration record incomplete")
	}
	return reg, nil
}

// bundledPackageHEFHash returns the sha256 of the HEF embedded in the AMPK
// package at path, computed fresh from the stream while the package digest is
// verified (a corrupted package is an error, never a hash). Used to compare a
// bundled package against a platform HEF: the container bytes would never
// match, the inner HEF bytes do.
func bundledPackageHEFHash(binPath string) (string, error) {
	f, err := os.Open(binPath)
	if err != nil {
		return "", fmt.Errorf("failed to open bundled package: %w", err)
	}
	defer f.Close()
	pr, err := storage.OpenPackage(f)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, pr.HEF()); err != nil {
		return "", fmt.Errorf("failed to read bundled HEF: %w", err)
	}
	if err := pr.Verify(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
