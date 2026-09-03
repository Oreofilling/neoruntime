package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	inferencepb "aipc/platform/ai-runtime/proto"
	"aipc/platform/app-manager/manifest"
	"aipc/platform/common/constants"
	"aipc/platform/platform-api/model"
	"aipc/platform/platform-api/storage"
)

// stubInferenceClient embeds the generated interface so only the methods
// implemented below exist; any other call would nil-panic, which is the
// desired signal that the code under test touched something unexpected.
type stubInferenceClient struct {
	inferencepb.InferenceServiceClient
	models        []*inferencepb.ModelInfo
	err           error
	registrations []*inferencepb.ModelRegisterRequest
	regErr        error                          // RPC-level error for RegisterModel
	regStatus     map[string]*inferencepb.Status // by model id: success=false body
	unregistered  []*inferencepb.ModelInfo
	infos         map[string]*inferencepb.ModelInfo // GetModelInfo results by model id
	inferCalls    []string                          // model ids probed via Infer
	inferFail     string                            // non-empty: Infer reports this failure
}

func (c *stubInferenceClient) ListModels(ctx context.Context, in *inferencepb.Empty, opts ...grpc.CallOption) (*inferencepb.ModelListResponse, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &inferencepb.ModelListResponse{Models: c.models}, nil
}

func (c *stubInferenceClient) RegisterModel(ctx context.Context, in *inferencepb.ModelRegisterRequest, opts ...grpc.CallOption) (*inferencepb.ModelRegisterResponse, error) {
	c.registrations = append(c.registrations, in)
	if c.regErr != nil {
		return nil, c.regErr
	}
	if st, ok := c.regStatus[in.ModelId]; ok {
		return &inferencepb.ModelRegisterResponse{ModelId: in.ModelId, Status: st}, nil
	}
	return &inferencepb.ModelRegisterResponse{ModelId: in.ModelId, Status: &inferencepb.Status{Success: true}}, nil
}

func (c *stubInferenceClient) UnregisterModel(ctx context.Context, in *inferencepb.ModelInfo, opts ...grpc.CallOption) (*inferencepb.Status, error) {
	c.unregistered = append(c.unregistered, in)
	return &inferencepb.Status{Success: true}, nil
}

// GetModelInfo serves the configured probe info; ids without an entry get an
// RPC error, which the preload smoke path treats as "no tensor info" and
// skips the probe — matching a runtime that cannot describe the model.
func (c *stubInferenceClient) GetModelInfo(ctx context.Context, in *inferencepb.ModelInfo, opts ...grpc.CallOption) (*inferencepb.ModelInfo, error) {
	if info, ok := c.infos[in.ModelId]; ok {
		return info, nil
	}
	return nil, errors.New("model not found")
}

func (c *stubInferenceClient) Infer(ctx context.Context, in *inferencepb.InferRequest, opts ...grpc.CallOption) (*inferencepb.InferResponse, error) {
	c.inferCalls = append(c.inferCalls, in.ModelId)
	if c.inferFail != "" {
		return &inferencepb.InferResponse{Status: &inferencepb.Status{Success: false, Message: c.inferFail}}, nil
	}
	return &inferencepb.InferResponse{Status: &inferencepb.Status{Success: true}}, nil
}

func newValidationServer(client inferencepb.InferenceServiceClient, enabled bool) *AppManagerServer {
	cfg := &Config{}
	cfg.AIRuntime.Enabled = enabled
	return &AppManagerServer{
		config:          cfg,
		aiRuntimeClient: client,
	}
}

func loadedModel(id string) *inferencepb.ModelInfo {
	return &inferencepb.ModelInfo{ModelId: id}
}

// withTempRoot points the global install prefix at a scratch directory so
// tests exercising appModelsDir never touch the real /data/aipc.
func withTempRoot(t *testing.T) string {
	t.Helper()
	old := constants.RootPath()
	root := t.TempDir()
	constants.SetRootPath(root)
	t.Cleanup(func() { constants.SetRootPath(old) })
	return root
}

// newModelMetaDB builds a throwaway sqlite db with the real ai_models schema
// getModelMeta reads (full AIModel rows — the load-time composition consumes
// output_mode/config/threshold/max_detections too).
func newModelMetaDB(t *testing.T, rows map[string]model.AIModel) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "platform.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AIModel{}); err != nil {
		t.Fatalf("AutoMigrate ai_models: %v", err)
	}
	for id, meta := range rows {
		row := meta
		row.ModelID = id
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	return db
}

func TestResolveModelDependencies(t *testing.T) {
	tests := []struct {
		name        string
		client      inferencepb.InferenceServiceClient
		enabled     bool
		models      map[string]manifest.ModelMapping
		dbRows      map[string]model.AIModel
		wantErr     string // empty = expect success
		wantWarning string // expected substring of the task message, empty = none
		wantPending []string
	}{
		{
			name:    "no_models_skips_rpc_entirely",
			client:  nil, // would fail the test if accessed
			enabled: false,
			models:  nil,
		},
		{
			name:    "required_model_loaded",
			client:  &stubInferenceClient{models: []*inferencepb.ModelInfo{loadedModel("clip_vit_b_32")}},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip": {ID: "clip_vit_b_32", Required: true},
			},
		},
		{
			name:    "model_in_platform_db_resolves_without_runtime_load",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "yolo_world_540", Required: true},
			},
			dbRows: map[string]model.AIModel{
				"yolo_world_540": {FilePath: "/data/aipc/models/yolo.hef", ModelType: "detection"},
			},
		},
		{
			name:    "db_hit_counts_even_when_runtime_unreachable",
			client:  nil,
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "yolo_world_540", Required: true},
			},
			dbRows: map[string]model.AIModel{
				"yolo_world_540": {FilePath: "/data/aipc/models/yolo.hef", ModelType: "detection"},
			},
		},
		{
			name:    "required_model_missing_fails",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "yolo_world_540", Required: true},
			},
			wantErr: `required model "yolo_world_540" (alias "detector") is not available on the device and no bundled path is declared`,
		},
		{
			name:    "optional_model_missing_warns_only",
			client:  &stubInferenceClient{models: []*inferencepb.ModelInfo{loadedModel("clip_vit_b_32")}},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip":    {ID: "clip_vit_b_32", Required: true},
				"detecto": {ID: "yolo_world_540"},
			},
			wantWarning: `optional model "yolo_world_540" (alias "detecto") is not available on the device and no bundled path is declared`,
		},
		{
			name:    "all_required_missing_errors_joined",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"zeta":  {ID: "model_z", Required: true},
				"alpha": {ID: "model_a", Required: true},
			},
			wantErr: `required model "model_a" (alias "alpha") is not available on the device and no bundled path is declared; required model "model_z" (alias "zeta") is not available on the device and no bundled path is declared`,
		},
		{
			name:    "id_miss_with_path_becomes_pending_no_error",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "bundled_det", Path: "/app/models/det.bin", Required: true},
			},
			wantPending: []string{"detector"},
		},
		{
			name:    "id_hit_beats_declared_path",
			client:  &stubInferenceClient{models: []*inferencepb.ModelInfo{loadedModel("bundled_det")}},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "bundled_det", Path: "/app/models/det.bin", Required: true},
			},
		},
		{
			name:    "pending_sorted_by_alias",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"zeta":  {ID: "model_z", Path: "/app/models/z.bin"},
				"alpha": {ID: "model_a", Path: "/app/models/a.bin"},
			},
			wantPending: []string{"alpha", "zeta"},
		},
		{
			name:    "client_nil_required_fails_unverified",
			client:  nil,
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip": {ID: "clip_vit_b_32", Required: true},
			},
			wantErr: `required model "clip_vit_b_32" (alias "clip") cannot be verified (ai-runtime is not available)`,
		},
		{
			name:    "client_nil_optional_warns_unverified",
			client:  nil,
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip": {ID: "clip_vit_b_32"},
			},
			wantWarning: `optional model "clip_vit_b_32" (alias "clip") cannot be verified`,
		},
		{
			name:    "rpc_error_fails_required",
			client:  &stubInferenceClient{err: errors.New("connection refused")},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip": {ID: "clip_vit_b_32", Required: true},
			},
			wantErr: "connection refused",
		},
		{
			name:    "runtime_disabled_treated_as_unavailable",
			client:  &stubInferenceClient{models: []*inferencepb.ModelInfo{loadedModel("clip_vit_b_32")}},
			enabled: false,
			models: map[string]manifest.ModelMapping{
				"clip": {ID: "clip_vit_b_32", Required: true},
			},
			wantErr: "ai-runtime is not available",
		},
		{
			name:    "nil_manifest_noop",
			client:  nil,
			enabled: false,
			models:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newValidationServer(tt.client, tt.enabled)
			if tt.dbRows != nil {
				s.db = newModelMetaDB(t, tt.dbRows)
			}
			appManifest := &manifest.AppManifest{Spec: manifest.Spec{Models: tt.models}}
			task := &InstallTask{ID: "test", Phase: "validating"}

			res, err := s.resolveModelDependencies(context.Background(), appManifest, task)
			if res == nil {
				t.Fatal("resolveModelDependencies() returned nil resolution")
			}

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("resolveModelDependencies() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("resolveModelDependencies() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("resolveModelDependencies() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}

			gotPending := make([]string, 0, len(res.pathPending))
			for _, p := range res.pathPending {
				gotPending = append(gotPending, p.alias)
			}
			if len(gotPending) != len(tt.wantPending) {
				t.Errorf("pathPending aliases = %v, want %v", gotPending, tt.wantPending)
			} else {
				for i := range gotPending {
					if gotPending[i] != tt.wantPending[i] {
						t.Errorf("pathPending aliases = %v, want %v", gotPending, tt.wantPending)
						break
					}
				}
			}

			if tt.wantWarning == "" {
				if task.Message != "" {
					t.Errorf("task.Message = %q, want untouched (empty)", task.Message)
				}
			} else {
				if !strings.Contains(task.Message, tt.wantWarning) {
					t.Errorf("task.Message = %q, want substring %q", task.Message, tt.wantWarning)
				}
				if !strings.HasPrefix(task.Message, "Warning: ") {
					t.Errorf("task.Message = %q, want Warning: prefix", task.Message)
				}
			}
		})
	}
}

func TestResolveModelDependenciesTaskOptional(t *testing.T) {
	s := newValidationServer(nil, true)
	appManifest := &manifest.AppManifest{Spec: manifest.Spec{Models: map[string]manifest.ModelMapping{
		"clip": {ID: "clip_vit_b_32"},
	}}}

	// nil task (sync InstallApp path): warning must log, not panic.
	if _, err := s.resolveModelDependencies(context.Background(), appManifest, nil); err != nil {
		t.Fatalf("resolveModelDependencies(nil task) unexpected error: %v", err)
	}

	// Attached task: progress message replaced with the warning.
	task := &InstallTask{ID: "t", Phase: "validating", Message: "Validation complete"}
	if _, err := s.resolveModelDependencies(context.Background(), appManifest, task); err != nil {
		t.Fatalf("resolveModelDependencies() unexpected error: %v", err)
	}
	_, _, message, _, _ := task.Snapshot()
	if !strings.HasPrefix(message, "Warning: ") {
		t.Errorf("task message = %q, want Warning: prefix", message)
	}
}

// fakeExtractor mimics containerd.Client.ExtractFileFromImage: materializes
// the "extracted" file under destDir and returns its path. Paths listed in
// fail fail with an error instead.
// fakeExtractor returns an extractModelFile stub that "extracts" genuine AMPK
// packages built with storage.WritePackage, so unpackBundledPackage sees a
// real digest-verified container rather than a fake byte blob. packages maps
// container path → package metadata; paths without an entry get the canonical
// detection package (the bundled flow's workhorse).
func fakeExtractor(packages map[string]*storage.PackageMeta, fail map[string]bool) func(ctx context.Context, imageRef, containerPath, destDir string) (string, error) {
	return func(ctx context.Context, imageRef, containerPath, destDir string) (string, error) {
		if fail[containerPath] {
			return "", errors.New("file not found in image")
		}
		meta := detectionPackageMeta("bundled_det")
		if m, ok := packages[containerPath]; ok {
			meta = m
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return "", err
		}
		p := filepath.Join(destDir, filepath.Base(containerPath))
		f, err := os.Create(p)
		if err != nil {
			return "", err
		}
		if err := storage.WritePackage(f, meta, bytes.NewReader([]byte("fake-hef-bytes"))); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return p, nil
	}
}

// detectionPackageMeta is the canonical bundled detection package: platform
// output mode, no config overrides — postprocess profile and thresholds come
// from the schema defaults at unpack time.
func detectionPackageMeta(id string) *storage.PackageMeta {
	return &storage.PackageMeta{
		ModelID:    id,
		ModelType:  "detection",
		OutputMode: "platform",
		Config:     json.RawMessage(`{}`),
		HEF:        storage.PackageHEF{Filename: id + ".hef"},
	}
}

func newExtractionServer(t *testing.T, client inferencepb.InferenceServiceClient, extractFn func(ctx context.Context, imageRef, containerPath, destDir string) (string, error)) *AppManagerServer {
	t.Helper()
	s := newValidationServer(client, true)
	s.extractModelFile = extractFn
	return s
}

func bundledImageManifest() *manifest.AppManifest {
	return &manifest.AppManifest{
		Metadata: manifest.Metadata{ID: "app-x", Name: "App X", Version: "1.0.0"},
		Spec: manifest.Spec{
			Image: "docker.io/library/app-x:1.0.0",
			Models: map[string]manifest.ModelMapping{
				"detector": {ID: "bundled_det", Path: "/app/models/det.bin", Required: true},
			},
		},
	}
}

func pendingFor(m *manifest.AppManifest, alias string) []pendingBundledModel {
	mp := m.Spec.Models[alias]
	return []pendingBundledModel{{alias: alias, id: mp.ID, path: mp.Path, required: mp.Required}}
}

func TestExtractImageModels(t *testing.T) {
	t.Run("success_registers_transient_model", func(t *testing.T) {
		root := withTempRoot(t)
		client := &stubInferenceClient{}
		s := newExtractionServer(t, client, fakeExtractor(nil, nil))
		m := bundledImageManifest()

		task := &InstallTask{ID: "t", Phase: "pulling"}
		if err := s.extractImageModels(context.Background(), "app-x", m, pendingFor(m, "detector"), task); err != nil {
			t.Fatalf("extractImageModels() unexpected error: %v", err)
		}

		if len(client.registrations) != 1 {
			t.Fatalf("registrations = %d, want 1", len(client.registrations))
		}
		reg := client.registrations[0]
		// Detection packages stage their HEF under the postprocess profile
		// basename (schema default profile: no config overrides in the
		// package), so modelload passes the file through without copying.
		wantPath := filepath.Join(root, "app-models", "app-x", "detector", model.DefaultDetectionProfile+".hef")
		if reg.ModelId != "bundled_det" || reg.ModelPath != wantPath || reg.OwnerId != "app-x" ||
			reg.ModelType != "detection" || !reg.Transient {
			t.Errorf("registration = %+v, want id=bundled_det path=%s owner=app-x type=detection transient=true", reg, wantPath)
		}
		if _, err := os.Stat(wantPath); err != nil {
			t.Errorf("unpacked HEF missing: %v", err)
		}
		// The registration sidecar (the record PreloadModels restores from
		// after a reboot) must have been written next to it.
		if _, err := os.Stat(filepath.Join(root, "app-models", "app-x", "detector", "registration.json")); err != nil {
			t.Errorf("registration sidecar missing: %v", err)
		}
		if len(client.unregistered) != 0 {
			t.Errorf("unregistered = %v, want none on success", client.unregistered)
		}
		if task.Phase != "registering" {
			t.Errorf("task.Phase = %q, want registering", task.Phase)
		}
	})

	t.Run("required_extract_failure_rolls_back", func(t *testing.T) {
		withTempRoot(t)
		client := &stubInferenceClient{}
		s := newExtractionServer(t, client, fakeExtractor(nil, map[string]bool{"/app/models/missing.bin": true}))
		m := bundledImageManifest()
		m.Spec.Models["broken"] = manifest.ModelMapping{ID: "bundled_broken", Path: "/app/models/missing.bin", Required: true}

		pending := append(pendingFor(m, "detector"), pendingFor(m, "broken")...)
		err := s.extractImageModels(context.Background(), "app-x", m, pending, nil)
		if err == nil {
			t.Fatal("extractImageModels() expected error for required extraction failure")
		}
		if !strings.Contains(err.Error(), `required model "bundled_broken" (alias "broken")`) {
			t.Errorf("error = %q, want bundled_broken/broken entry", err.Error())
		}

		// The first model was registered before the second failed: rollback
		// must unregister it and remove the extraction directory.
		if len(client.registrations) != 1 || client.registrations[0].ModelId != "bundled_det" {
			t.Fatalf("registrations = %+v, want only bundled_det", client.registrations)
		}
		if len(client.unregistered) != 1 || client.unregistered[0].ModelId != "bundled_det" {
			t.Errorf("unregistered = %+v, want bundled_det", client.unregistered)
		}
		if _, statErr := os.Stat(appModelsDir("app-x")); !os.IsNotExist(statErr) {
			t.Errorf("extraction dir still present after rollback (stat err=%v)", statErr)
		}
	})

	t.Run("optional_failure_warns_and_continues", func(t *testing.T) {
		withTempRoot(t)
		client := &stubInferenceClient{}
		s := newExtractionServer(t, client, fakeExtractor(nil, map[string]bool{"/app/models/opt.bin": true}))
		m := bundledImageManifest()
		m.Spec.Models["extra"] = manifest.ModelMapping{ID: "bundled_opt", Path: "/app/models/opt.bin"}

		pending := append(pendingFor(m, "detector"), pendingFor(m, "extra")...)
		task := &InstallTask{ID: "t", Phase: "pulling"}
		if err := s.extractImageModels(context.Background(), "app-x", m, pending, task); err != nil {
			t.Fatalf("extractImageModels() unexpected error: %v", err)
		}
		if len(client.registrations) != 1 || client.registrations[0].ModelId != "bundled_det" {
			t.Fatalf("registrations = %+v, want only bundled_det", client.registrations)
		}
		if !strings.Contains(task.Message, `optional model "bundled_opt" (alias "extra")`) {
			t.Errorf("task.Message = %q, want optional bundled_opt warning", task.Message)
		}
	})

	t.Run("register_status_failure_fails_required", func(t *testing.T) {
		withTempRoot(t)
		client := &stubInferenceClient{regStatus: map[string]*inferencepb.Status{
			"bundled_det": {Success: false, Message: "model_type is required for app-bundled model"},
		}}
		s := newExtractionServer(t, client, fakeExtractor(nil, nil))
		m := bundledImageManifest()

		err := s.extractImageModels(context.Background(), "app-x", m, pendingFor(m, "detector"), nil)
		if err == nil {
			t.Fatal("extractImageModels() expected error when runtime rejects the registration")
		}
		if !strings.Contains(err.Error(), "model_type is required for app-bundled model") {
			t.Errorf("error = %q, want runtime status message included", err.Error())
		}
		if _, statErr := os.Stat(appModelsDir("app-x")); !os.IsNotExist(statErr) {
			t.Errorf("extraction dir still present after rollback (stat err=%v)", statErr)
		}
	})

	t.Run("required_smoke_failure_fails_install", func(t *testing.T) {
		withTempRoot(t)
		// Probe inputs make the smoke test actually run an Infer frame;
		// inferFail is the postprocess-mismatch scenario the probe exists for.
		client := &stubInferenceClient{
			infos:     map[string]*inferencepb.ModelInfo{"bundled_det": probeInputs("bundled_det")},
			inferFail: "output tensor count mismatch",
		}
		s := newExtractionServer(t, client, fakeExtractor(nil, nil))
		m := bundledImageManifest()

		err := s.extractImageModels(context.Background(), "app-x", m, pendingFor(m, "detector"), nil)
		if err == nil {
			t.Fatal("extractImageModels() expected error when the smoke probe fails")
		}
		if !strings.Contains(err.Error(), "postprocess smoke test failed") {
			t.Errorf("error = %q, want smoke failure reason", err.Error())
		}
		// The registration was attempted, probed, and rolled back — and the
		// unpacked files are gone so PreloadModels cannot resurrect them.
		if len(client.registrations) != 1 || client.registrations[0].ModelId != "bundled_det" {
			t.Fatalf("registrations = %+v, want the attempted bundled_det registration", client.registrations)
		}
		if len(client.inferCalls) != 1 || client.inferCalls[0] != "bundled_det" {
			t.Errorf("inferCalls = %v, want one bundled_det probe", client.inferCalls)
		}
		if len(client.unregistered) != 1 || client.unregistered[0].ModelId != "bundled_det" {
			t.Errorf("unregistered = %+v, want bundled_det rolled back", client.unregistered)
		}
		if _, statErr := os.Stat(appModelsDir("app-x")); !os.IsNotExist(statErr) {
			t.Errorf("extraction dir still present after smoke rollback (stat err=%v)", statErr)
		}
	})

	t.Run("optional_smoke_failure_warns_and_skips", func(t *testing.T) {
		withTempRoot(t)
		// The required detector gets no tensor info (GetModelInfo errors →
		// probe skipped); only the optional model is probed and fails.
		client := &stubInferenceClient{
			infos:     map[string]*inferencepb.ModelInfo{"bundled_opt": probeInputs("bundled_opt")},
			inferFail: "output tensor count mismatch",
		}
		s := newExtractionServer(t, client, fakeExtractor(map[string]*storage.PackageMeta{
			"/app/models/opt.bin": detectionPackageMeta("bundled_opt"),
		}, nil))
		m := bundledImageManifest()
		m.Spec.Models["extra"] = manifest.ModelMapping{ID: "bundled_opt", Path: "/app/models/opt.bin"}

		pending := append(pendingFor(m, "detector"), pendingFor(m, "extra")...)
		task := &InstallTask{ID: "t", Phase: "pulling"}
		if err := s.extractImageModels(context.Background(), "app-x", m, pending, task); err != nil {
			t.Fatalf("extractImageModels() unexpected error: %v", err)
		}
		if len(client.registrations) != 2 {
			t.Fatalf("registrations = %d, want 2 attempts", len(client.registrations))
		}
		if len(client.inferCalls) != 1 || client.inferCalls[0] != "bundled_opt" {
			t.Errorf("inferCalls = %v, want only the bundled_opt probe", client.inferCalls)
		}
		if !strings.Contains(task.Message, `optional model "bundled_opt" (alias "extra")`) {
			t.Errorf("task.Message = %q, want optional bundled_opt warning", task.Message)
		}
		if !strings.Contains(task.Message, "postprocess smoke test failed") {
			t.Errorf("task.Message = %q, want smoke failure in the warning", task.Message)
		}
		// The optional model's registration was rolled back; the required
		// detector stays registered.
		if len(client.unregistered) != 1 || client.unregistered[0].ModelId != "bundled_opt" {
			t.Errorf("unregistered = %+v, want only bundled_opt", client.unregistered)
		}
	})

	t.Run("nil_extractor_reports_unavailable", func(t *testing.T) {
		s := newExtractionServer(t, &stubInferenceClient{}, nil)
		m := bundledImageManifest()

		err := s.extractImageModels(context.Background(), "app-x", m, pendingFor(m, "detector"), nil)
		if err == nil {
			t.Fatal("extractImageModels() expected error without an extractor")
		}
		if !strings.Contains(err.Error(), "cannot be extracted (containerd is not available)") {
			t.Errorf("error = %q, want containerd-unavailable reason", err.Error())
		}
	})

	t.Run("no_pending_noop", func(t *testing.T) {
		s := newValidationServer(nil, false) // no client, no extractor: must not matter
		if err := s.extractImageModels(context.Background(), "app-x", bundledImageManifest(), nil, nil); err != nil {
			t.Fatalf("extractImageModels(empty) unexpected error: %v", err)
		}
	})
}

func TestPreloadModelsRestoresBundledModels(t *testing.T) {
	root := withTempRoot(t)
	extracted := filepath.Join(root, "app-models", "app-x", "detector", "det.hef")
	if err := os.MkdirAll(filepath.Dir(extracted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extracted, []byte("fake-hef"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The sidecar install time wrote next to the unpacked HEF: it carries the
	// composed gRPC registration the .bin itself is gone too far to re-derive.
	sidecar := `{"model_id": "bundled_det", "hef": "det.hef", "model_type": "detection"}`
	if err := os.WriteFile(filepath.Join(filepath.Dir(extracted), "registration.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &stubInferenceClient{}
	s := newValidationServer(client, true)
	appManifest := &manifest.AppManifest{
		Spec: manifest.Spec{
			Models: map[string]manifest.ModelMapping{
				"detector": {ID: "bundled_det", Path: "/app/models/det.bin"},
			},
			Permissions: manifest.Permissions{
				Inference: manifest.InferencePerms{Models: []string{"bundled_det", "platform_only"}},
			},
		},
	}

	s.PreloadModels(context.Background(), "app-x", appManifest)

	// bundled_det: not in platform.db, restored from the extracted file as a
	// transient model. platform_only: neither in db nor bundled — no
	// registration, just a logged warning.
	if len(client.registrations) != 1 {
		t.Fatalf("registrations = %+v, want exactly the bundled one", client.registrations)
	}
	reg := client.registrations[0]
	if reg.ModelId != "bundled_det" || reg.ModelPath != extracted || reg.OwnerId != "app-x" ||
		reg.ModelType != "detection" || !reg.Transient {
		t.Errorf("registration = %+v, want id=bundled_det path=%s owner=app-x type=detection transient=true", reg, extracted)
	}
}

func TestPreloadModelsBundledFileMissing(t *testing.T) {
	root := withTempRoot(t)
	// Sidecar present, HEF gone (a partial wipe): the restore must warn and
	// register nothing rather than point a registration at a missing file.
	aliasDir := filepath.Join(root, "app-models", "app-x", "detector")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := `{"model_id": "bundled_det", "hef": "det.hef", "model_type": "detection"}`
	if err := os.WriteFile(filepath.Join(aliasDir, "registration.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &stubInferenceClient{}
	s := newValidationServer(client, true)
	appManifest := &manifest.AppManifest{
		Spec: manifest.Spec{
			Models: map[string]manifest.ModelMapping{
				"detector": {ID: "bundled_det", Path: "/app/models/det.bin"},
			},
			Permissions: manifest.Permissions{
				Inference: manifest.InferencePerms{Models: []string{"bundled_det"}},
			},
		},
	}

	s.PreloadModels(context.Background(), "app-x", appManifest)

	if len(client.registrations) != 0 {
		t.Errorf("registrations = %+v, want none when the extracted file is missing", client.registrations)
	}
}

func TestUnloadModelsRemovesExtractedDir(t *testing.T) {
	root := withTempRoot(t)
	dir := filepath.Join(root, "app-models", "app-x", "detector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "det.hef"), []byte("fake-hef"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Nil ai-runtime client: the runtime sweep is skipped, but the extracted
	// files must still be removed.
	s := newValidationServer(nil, false)
	s.UnloadModels(context.Background(), "app-x", "")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("extracted dir still present after unload (stat err=%v)", err)
	}
}

// contentExtractor mimics containerd extraction of an AMPK package whose
// embedded HEF bytes are exactly content, so hash comparisons against a
// platform HEF are deterministic (the shadowed-model check hashes the inner
// HEF, not the container).
func contentExtractor(content []byte) func(ctx context.Context, imageRef, containerPath, destDir string) (string, error) {
	return func(ctx context.Context, imageRef, containerPath, destDir string) (string, error) {
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return "", err
		}
		p := filepath.Join(destDir, filepath.Base(containerPath))
		f, err := os.Create(p)
		if err != nil {
			return "", err
		}
		meta := &storage.PackageMeta{HEF: storage.PackageHEF{Filename: "inner.hef"}}
		if err := storage.WritePackage(f, meta, bytes.NewReader(content)); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return p, nil
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func assertNoShadowTempLeak(t *testing.T) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(os.TempDir(), "aipc-shadowed-*"))
	if err != nil {
		return // glob trouble is not the code under test
	}
	if len(leftovers) != 0 {
		t.Errorf("shadowed-model temp dirs leaked: %v", leftovers)
	}
}

func TestResolveModelDependenciesShadowedCapture(t *testing.T) {
	client := &stubInferenceClient{models: []*inferencepb.ModelInfo{
		{ModelId: "runtime_det", ModelPath: "/data/aipc/models/runtime.hef"},
	}}
	s := newValidationServer(client, true)
	s.db = newModelMetaDB(t, map[string]model.AIModel{
		"db_det": {FilePath: "/data/aipc/models/db.hef", ModelType: "detection"},
	})
	m := &manifest.AppManifest{Spec: manifest.Spec{Models: map[string]manifest.ModelMapping{
		"rt":    {ID: "runtime_det", Path: "/app/models/rt.bin", Required: true},
		"dbm":   {ID: "db_det", Path: "/app/models/db.bin", Required: true},
		"plain": {ID: "runtime_det"}, // id hit without a path: resolved, never shadowed
	}}}

	res, err := s.resolveModelDependencies(context.Background(), m, nil)
	if err != nil {
		t.Fatalf("resolveModelDependencies() unexpected error: %v", err)
	}
	if len(res.pathPending) != 0 {
		t.Errorf("pathPending = %+v, want none (both ids hit)", res.pathPending)
	}
	if len(res.resolved) != 3 {
		t.Errorf("resolved = %v, want all 3 ids", res.resolved)
	}
	want := map[string]string{
		"rt":  "/data/aipc/models/runtime.hef",
		"dbm": "/data/aipc/models/db.hef",
	}
	if len(res.shadowed) != len(want) {
		t.Fatalf("shadowed = %+v, want %d entries", res.shadowed, len(want))
	}
	for _, sh := range res.shadowed {
		if sh.platformPath != want[sh.alias] {
			t.Errorf("shadowed[%s].platformPath = %q, want %q", sh.alias, sh.platformPath, want[sh.alias])
		}
		if sh.id == "" || sh.path == "" {
			t.Errorf("shadowed[%s] missing id or path: %+v", sh.alias, sh)
		}
	}
}

func TestCheckShadowedModels(t *testing.T) {
	m := bundledImageManifest() // supplies ImageReferences()

	t.Run("hash_mismatch_warns", func(t *testing.T) {
		platform := writeTempFile(t, "platform.hef", "platform-version")
		s := newExtractionServer(t, &stubInferenceClient{}, contentExtractor([]byte("bundled-version")))
		task := &InstallTask{ID: "t", Phase: "pulling"}

		s.checkShadowedModels(context.Background(), m, []shadowedBundledModel{
			{alias: "detector", id: "shared_det", path: "/app/models/det.bin", platformPath: platform},
		}, task)

		if !strings.Contains(task.Message, "平台已有同 id 模型，镜像内版本被忽略") {
			t.Errorf("task.Message = %q, want shadowing warning", task.Message)
		}
		if !strings.Contains(task.Message, `model "shared_det" (alias "detector")`) {
			t.Errorf("task.Message = %q, want id/alias context", task.Message)
		}
		if !strings.HasPrefix(task.Message, "Warning: ") {
			t.Errorf("task.Message = %q, want Warning: prefix", task.Message)
		}
		assertNoShadowTempLeak(t)
	})

	t.Run("hash_match_is_quiet", func(t *testing.T) {
		platform := writeTempFile(t, "platform.hef", "same-version")
		s := newExtractionServer(t, &stubInferenceClient{}, contentExtractor([]byte("same-version")))
		task := &InstallTask{ID: "t", Phase: "pulling"}

		s.checkShadowedModels(context.Background(), m, []shadowedBundledModel{
			{alias: "detector", id: "shared_det", path: "/app/models/det.bin", platformPath: platform},
		}, task)

		if task.Message != "" {
			t.Errorf("task.Message = %q, want untouched on hash match", task.Message)
		}
		assertNoShadowTempLeak(t)
	})

	t.Run("platform_file_unreadable_skips", func(t *testing.T) {
		s := newExtractionServer(t, &stubInferenceClient{}, contentExtractor([]byte("bundled")))
		task := &InstallTask{ID: "t", Phase: "pulling"}

		s.checkShadowedModels(context.Background(), m, []shadowedBundledModel{
			{alias: "detector", id: "shared_det", path: "/app/models/det.bin", platformPath: filepath.Join(t.TempDir(), "gone.hef")},
		}, task)

		if task.Message != "" {
			t.Errorf("task.Message = %q, want no warning when the platform copy is unreadable", task.Message)
		}
	})

	t.Run("image_copy_unreadable_skips", func(t *testing.T) {
		platform := writeTempFile(t, "platform.hef", "platform-version")
		s := newExtractionServer(t, &stubInferenceClient{}, fakeExtractor(nil, map[string]bool{"/app/models/det.bin": true}))
		task := &InstallTask{ID: "t", Phase: "pulling"}

		s.checkShadowedModels(context.Background(), m, []shadowedBundledModel{
			{alias: "detector", id: "shared_det", path: "/app/models/det.bin", platformPath: platform},
		}, task)

		if task.Message != "" {
			t.Errorf("task.Message = %q, want no warning when the bundled copy is unreadable", task.Message)
		}
		assertNoShadowTempLeak(t)
	})

	t.Run("no_extractor_skips", func(t *testing.T) {
		s := newValidationServer(&stubInferenceClient{}, true) // no extractModelFile wired
		task := &InstallTask{ID: "t", Phase: "pulling"}

		s.checkShadowedModels(context.Background(), m, []shadowedBundledModel{
			{alias: "detector", id: "shared_det", path: "/app/models/det.bin", platformPath: "/data/models/x.hef"},
		}, task)

		if task.Message != "" {
			t.Errorf("task.Message = %q, want no warning without containerd extraction", task.Message)
		}
	})
}
