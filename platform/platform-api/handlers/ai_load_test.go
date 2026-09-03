package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"aipc/platform/common/constants"
	"aipc/platform/platform-api/model"

	inferencepb "aipc/platform/ai-runtime/proto"
)

// postModelAction drives the load/unload endpoints without a live gin engine.
func postModelAction(t *testing.T, h *APIHandlers, action, modelID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "model_id", Value: modelID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/models/"+modelID+"/"+action, nil)
	if action == "load" {
		h.LoadModel(c)
	} else {
		h.UnloadModel(c)
	}
	return w
}

// A row left "loaded" by a solo ai-runtime restart must not deadlock: load
// heals the stale status and actually registers the model again.
func TestLoadModelHealsStaleLoadedRow(t *testing.T) {
	h, fake, store := newAIUpdateTestEnv(t)
	oldRoot := constants.RootPath()
	constants.SetRootPath(t.TempDir())
	t.Cleanup(func() { constants.SetRootPath(oldRoot) })
	blob := seedBlob(t, store, "h1")
	seedAIModel(t, h, &model.AIModel{
		ModelID: "stale_det", Name: "stale_det", Status: "loaded", Source: "web",
		ModelType: "detection", FilePath: blob, FileHash: "h1", DesiredState: "loaded",
	})

	w := postModelAction(t, h, "load", "stale_det")
	if respCode(t, w) != 0 {
		t.Fatalf("stale-loaded model must be loadable, got: %s", w.Body.String())
	}
	calls, _ := fake.snapshot()
	if len(calls) != 1 || calls[0] != "load:stale_det" {
		t.Fatalf("expected exactly one register call, got %v", calls)
	}
	row, _ := h.aiModelRepo.GetByModelID("stale_det")
	if row.Status != "loaded" || row.DesiredState != "loaded" {
		t.Errorf("row after heal+load: status=%q desired=%q, want loaded/loaded", row.Status, row.DesiredState)
	}
}

// Runtime serving a model the DB calls "uploaded": load rejects with the
// usual message but heals the row so the two views agree.
func TestLoadModelRejectsButHealsWhenRuntimeServesIt(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	fake.markLive("ghost_live")
	seedAIModel(t, h, &model.AIModel{
		ModelID: "ghost_live", Name: "ghost_live", Status: "uploaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h1.hef",
	})

	w := postModelAction(t, h, "load", "ghost_live")
	if respCode(t, w) != CodeInvalidRequest || !strings.Contains(w.Body.String(), "already loaded") {
		t.Fatalf("must reject with already-loaded, got: %s", w.Body.String())
	}
	if calls, _ := fake.snapshot(); len(calls) != 0 {
		t.Errorf("no runtime mutation expected, got %v", calls)
	}
	row, _ := h.aiModelRepo.GetByModelID("ghost_live")
	if row.Status != "loaded" || row.DesiredState != "loaded" {
		t.Errorf("row must be healed to loaded, got status=%q desired=%q", row.Status, row.DesiredState)
	}
}

// Unload of a stale-loaded row: same "not loaded" response as before, but
// the DB row is healed so a subsequent load is not rejected.
func TestUnloadModelHealsStaleLoadedRow(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	seedAIModel(t, h, &model.AIModel{
		ModelID: "stale_unload", Name: "stale_unload", Status: "loaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h1.hef", DesiredState: "loaded",
	})

	w := postModelAction(t, h, "unload", "stale_unload")
	if respCode(t, w) != CodeInvalidRequest || !strings.Contains(w.Body.String(), "not loaded") {
		t.Fatalf("unload of missing model must still respond not-loaded, got: %s", w.Body.String())
	}
	row, _ := h.aiModelRepo.GetByModelID("stale_unload")
	if row.Status != "uploaded" || row.DesiredState != "unloaded" {
		t.Errorf("row must heal to uploaded/unloaded, got %q/%q", row.Status, row.DesiredState)
	}

	// The healed row is now loadable — the deadlock is gone.
	if calls, _ := fake.snapshot(); len(calls) != 0 {
		t.Fatalf("no runtime calls expected, got %v", calls)
	}
}

// Normal unload still works end to end against the live set.
func TestUnloadModelLiveRowUnregisters(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	fake.markLive("live_det")
	seedAIModel(t, h, &model.AIModel{
		ModelID: "live_det", Name: "live_det", Status: "loaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h1.hef", DesiredState: "loaded",
	})

	w := postModelAction(t, h, "unload", "live_det")
	if respCode(t, w) != 0 {
		t.Fatalf("unload failed: %s", w.Body.String())
	}
	calls, _ := fake.snapshot()
	if len(calls) != 1 || calls[0] != "unload:live_det" {
		t.Fatalf("expected one unregister call, got %v", calls)
	}
	row, _ := h.aiModelRepo.GetByModelID("live_det")
	if row.Status != "uploaded" || row.DesiredState != "unloaded" {
		t.Errorf("row after unload: %q/%q, want uploaded/unloaded", row.Status, row.DesiredState)
	}
}

// A legacy preload registration handed the runtime the raw CAS blob path
// with no variant — the silent-degradation live trap this change closes the
// loop on. Load detects the path cannot be the composed registration (path is
// the only signal ModelInfo exposes), unregisters, and re-registers under the
// materialized profile basename with the full-key variant blob.
func TestLoadModelHealsLegacyBareBlobRegistration(t *testing.T) {
	h, fake, store := newAIUpdateTestEnv(t)
	oldRoot := constants.RootPath()
	root := t.TempDir()
	constants.SetRootPath(root)
	t.Cleanup(func() { constants.SetRootPath(oldRoot) })
	blob := seedBlob(t, store, "h1")
	seedAIModel(t, h, &model.AIModel{
		ModelID: "legacy_det", Name: "legacy_det", Status: "loaded", Source: "web",
		ModelType: "detection", FilePath: blob, FileHash: "h1", DesiredState: "loaded",
	})
	// The real runtime reports ownerless registrations as "<system>", never
	// "" — seed the exact shape a bare gRPC/legacy-preload registration has
	// on device.
	fake.markLiveEntry(&inferencepb.ModelInfo{ModelId: "legacy_det", ModelPath: blob, OwnerId: systemOwnerID})

	w := postModelAction(t, h, "load", "legacy_det")
	if respCode(t, w) != 0 {
		t.Fatalf("legacy bare-blob registration must heal via reload, got: %s", w.Body.String())
	}
	calls, _ := fake.snapshot()
	if len(calls) != 2 || calls[0] != "unload:legacy_det" || calls[1] != "load:legacy_det" {
		t.Fatalf("expected unload+reload heal, got %v", calls)
	}
	wantPath := filepath.Join(root, "models", "runtime", "legacy_det", "hailo_yolov8n_384_640.hef")
	if got := fake.registeredPath("legacy_det"); got != wantPath {
		t.Fatalf("re-registered path = %q, want materialized %q", got, wantPath)
	}
	variant := fake.registeredVariant("legacy_det")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(variant), &parsed); err != nil {
		t.Fatalf("re-registered variant is not JSON: %v (%q)", err, variant)
	}
	if parsed["backend_function"] != "hailo_yolov8n" {
		t.Fatalf("variant backend_function = %v, want hailo_yolov8n", parsed["backend_function"])
	}
	if _, ok := parsed["detection_threshold"]; !ok {
		t.Fatalf("variant must carry the full plugin schema keys, got %v", parsed)
	}
}

// App-owned entries are somebody's live registration, not residue: an owner
// mismatched path must not be healed away.
func TestLoadModelDoesNotHealAppOwnedRegistration(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	fake.markLiveEntry(&inferencepb.ModelInfo{ModelId: "app_det", ModelPath: "/elsewhere/x.hef", OwnerId: "app-1"})
	seedAIModel(t, h, &model.AIModel{
		ModelID: "app_det", Name: "app_det", Status: "uploaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h1.hef", FileHash: "h1",
	})

	w := postModelAction(t, h, "load", "app_det")
	if respCode(t, w) != CodeInvalidRequest || !strings.Contains(w.Body.String(), "already loaded") {
		t.Fatalf("owned registration must stay already-loaded, got: %s", w.Body.String())
	}
	if calls, _ := fake.snapshot(); len(calls) != 0 {
		t.Errorf("no runtime mutation expected, got %v", calls)
	}
}

// Transient registrations (bundled models) are equally not heal targets.
func TestLoadModelDoesNotHealTransientRegistration(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	fake.markLiveEntry(&inferencepb.ModelInfo{ModelId: "tmp_det", ModelPath: "/elsewhere/y.hef", Transient: true})
	seedAIModel(t, h, &model.AIModel{
		ModelID: "tmp_det", Name: "tmp_det", Status: "uploaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h2.hef", FileHash: "h2",
	})

	w := postModelAction(t, h, "load", "tmp_det")
	if respCode(t, w) != CodeInvalidRequest || !strings.Contains(w.Body.String(), "already loaded") {
		t.Fatalf("transient registration must stay already-loaded, got: %s", w.Body.String())
	}
	if calls, _ := fake.snapshot(); len(calls) != 0 {
		t.Errorf("no runtime mutation expected, got %v", calls)
	}
}

// Startup reconciliation: rows claiming loaded while the runtime is empty
// flip to uploaded, with DesiredState preserved for recovery.
func TestReconcileRuntimeModelsHealsStaleRows(t *testing.T) {
	h, fake, _ := newAIUpdateTestEnv(t)
	seedAIModel(t, h, &model.AIModel{
		ModelID: "stale_boot", Name: "stale_boot", Status: "loaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h1.hef", DesiredState: "loaded",
	})
	seedAIModel(t, h, &model.AIModel{
		ModelID: "parked", Name: "parked", Status: "uploaded", Source: "web",
		ModelType: "detection", FilePath: "/blobs/h2.hef",
	})

	h.ReconcileRuntimeModels()

	stale, _ := h.aiModelRepo.GetByModelID("stale_boot")
	if stale.Status != "uploaded" {
		t.Errorf("stale row status = %q, want uploaded", stale.Status)
	}
	if stale.DesiredState != "loaded" {
		t.Errorf("desired state must survive reconciliation (reload intent), got %q", stale.DesiredState)
	}
	parked, _ := h.aiModelRepo.GetByModelID("parked")
	if parked.Status != "uploaded" {
		t.Errorf("parked row must stay uploaded, got %q", parked.Status)
	}
	if calls, _ := fake.snapshot(); len(calls) != 0 {
		t.Errorf("reconciliation must not touch the runtime, got %v", calls)
	}
}
