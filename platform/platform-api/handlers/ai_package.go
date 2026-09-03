package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"aipc/platform/common/logger"
	"aipc/platform/platform-api/model"
	"aipc/platform/platform-api/storage"
)

// AMPK package import/export: the single-file .bin transport that carries a
// HEF plus the platform registration metadata (see storage/modelpackage.go
// for the container layout and its security boundaries).

// importModelPackage validates an uploaded AMPK package and stages the inner
// HEF into CAS. It makes two passes over the (seekable) multipart file: pass
// one proves the package digest end-to-end before anything is staged, pass
// two streams the HEF into the blob store. The blob is named by the HEF's
// own sha256, so package imports dedup with plain .hef imports of the same
// model bytes.
func (h *APIHandlers) importModelPackage(file multipart.File) (*storage.PackageMeta, *storage.SaveResult, error) {
	// Pass 1: full digest check — nothing is staged from an unverified package.
	pr, err := storage.OpenPackage(file)
	if err != nil {
		return nil, nil, err
	}
	meta := pr.Meta()
	if _, err := io.Copy(io.Discard, pr.HEF()); err != nil {
		return nil, nil, fmt.Errorf("failed to read package HEF section: %w", err)
	}
	if err := pr.Verify(); err != nil {
		return nil, nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("failed to rewind upload: %w", err)
	}

	// Pass 2: stage the verified HEF bytes.
	pr, err = storage.OpenPackage(file)
	if err != nil {
		return nil, nil, err
	}
	result, err := h.modelStore.SaveWithHash(pr.HEF(), ".hef")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stage package HEF: %w", err)
	}
	// The metadata may pin the HEF content hash; a mismatch means the package
	// lied about its payload. Only a freshly created blob is deleted — a
	// dedup hit is shared with existing rows and must stay.
	if meta.HEF.SHA256 != "" && meta.HEF.SHA256 != result.Hash {
		if !result.Existed {
			h.modelStore.Delete(result.Hash, ".hef")
		}
		return nil, nil, fmt.Errorf("package HEF sha256 mismatch (metadata says %s, payload hashes to %s)", meta.HEF.SHA256, result.Hash)
	}
	return meta, result, nil
}

// effectiveExportConfig composes the config section for export: the stored
// schema-driven config, with the authoritative column values winning for
// detection rows (threshold / max_detections are what the composed runtime
// variant actually consumes), so the package reproduces runtime behavior.
func effectiveExportConfig(m *model.AIModel) json.RawMessage {
	cfg := map[string]interface{}{}
	if m.Config != "" {
		_ = json.Unmarshal([]byte(m.Config), &cfg) // stale config falls back to columns
	}
	if model.ResolveModelType(m.ModelType) == "detection" {
		if m.Threshold > 0 {
			cfg["threshold"] = m.Threshold
		}
		if m.MaxDetections > 0 {
			cfg["max_detections"] = m.MaxDetections
		}
	}
	if len(cfg) == 0 {
		return nil
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	return blob
}

// ExportModel streams a registered device-level model as a single AMPK .bin
// package: the row's effective configuration as strict JSON metadata plus the
// HEF bytes byte-identical to the stored file. Parsing the package on another
// device (ParseModel's package path) reproduces the registration.
func (h *APIHandlers) ExportModel(c *gin.Context) {
	modelID := c.Param("model_id")
	if modelID == "" {
		Resp(c).FailMsg(CodeInvalidRequest, "Model ID is required")
		return
	}
	if h.aiModelRepo == nil {
		Resp(c).FailMsg(CodeServiceError, "Model repository not available")
		return
	}

	m, err := h.aiModelRepo.GetByModelID(modelID)
	if err != nil || m == nil || m.OwnerAppID != "" || m.FilePath == "" {
		// App-origin rows are not device-level models — same as not found.
		Resp(c).FailMsg(CodeNotFound, "Model not found")
		return
	}

	hef, err := os.Open(m.FilePath)
	if err != nil {
		Resp(c).FailMsg(CodeServiceError, "Failed to open model file: "+err.Error())
		return
	}
	defer hef.Close()

	// Hash the payload up front so the package states the file's true digest
	// even if the row's FileHash has gone stale, then rewind for WritePackage
	// (which reads the HEF twice: digest pass and copy pass).
	hasher := sha256.New()
	if _, err := io.Copy(hasher, hef); err != nil {
		Resp(c).FailMsg(CodeServiceError, "Failed to read model file: "+err.Error())
		return
	}
	if _, err := hef.Seek(0, io.SeekStart); err != nil {
		Resp(c).FailMsg(CodeServiceError, "Failed to rewind model file: "+err.Error())
		return
	}

	outputMode, _ := model.ResolveOutputMode(m.OutputMode)
	meta := &storage.PackageMeta{
		ModelID:      m.ModelID,
		Name:         m.Name,
		ModelType:    m.ModelType,
		OutputMode:   outputMode,
		Config:       effectiveExportConfig(m),
		HEF:          storage.PackageHEF{Filename: filepath.Base(m.FilePath), SHA256: hex.EncodeToString(hasher.Sum(nil))},
		Network:      storage.PackageNetwork{Name: m.NetworkName, InputWidth: m.InputWidth, InputHeight: m.InputHeight},
		OutputFormat: model.ClassifyOutputFormat(m.VStreamInfo),
	}

	c.Header("Content-Disposition", `attachment; filename="`+url.PathEscape(modelID)+".bin"+`"`)
	c.Header("Content-Type", "application/octet-stream")
	c.Status(http.StatusOK)
	if err := storage.WritePackage(c.Writer, meta, hef); err != nil {
		// Headers are already on the wire; the client sees a short body. Log
		// for the operator rather than failing an unsendable response.
		logger.Warn("ExportModel %s: package stream failed after headers: %v", modelID, err)
	}
}
