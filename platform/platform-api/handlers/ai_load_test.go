package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"aipc/platform/common/constants"
	"aipc/platform/platform-api/model"
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
