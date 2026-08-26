package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"

	inferencepb "aipc/platform/ai-runtime/proto"
	"aipc/platform/app-manager/manifest"
)

// stubInferenceClient embeds the generated interface so only ListModels is
// implemented; any other call would nil-panic, which is the desired signal
// that validateModelDependencies touched something unexpected.
type stubInferenceClient struct {
	inferencepb.InferenceServiceClient
	models []*inferencepb.ModelInfo
	err    error
}

func (c *stubInferenceClient) ListModels(ctx context.Context, in *inferencepb.Empty, opts ...grpc.CallOption) (*inferencepb.ModelListResponse, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &inferencepb.ModelListResponse{Models: c.models}, nil
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

func TestValidateModelDependencies(t *testing.T) {
	tests := []struct {
		name        string
		client      inferencepb.InferenceServiceClient
		enabled     bool
		models      map[string]manifest.ModelMapping
		wantErr     string // empty = expect success
		wantWarning string // expected substring of the task message, empty = none
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
			name:    "required_model_missing_fails",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"detector": {ID: "yolo_world_540", Required: true},
			},
			wantErr: `required model "yolo_world_540" (alias "detector") is not loaded on the device`,
		},
		{
			name:    "optional_model_missing_warns_only",
			client:  &stubInferenceClient{models: []*inferencepb.ModelInfo{loadedModel("clip_vit_b_32")}},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"clip":    {ID: "clip_vit_b_32", Required: true},
				"detecto": {ID: "yolo_world_540"},
			},
			wantWarning: `optional model "yolo_world_540" (alias "detecto") is not loaded on the device`,
		},
		{
			name:    "all_required_missing_errors_joined",
			client:  &stubInferenceClient{},
			enabled: true,
			models: map[string]manifest.ModelMapping{
				"zeta":  {ID: "model_z", Required: true},
				"alpha": {ID: "model_a", Required: true},
			},
			wantErr: `required model "model_a" (alias "alpha") is not loaded on the device; required model "model_z" (alias "zeta") is not loaded on the device`,
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
			appManifest := &manifest.AppManifest{Spec: manifest.Spec{Models: tt.models}}
			task := &InstallTask{ID: "test", Phase: "validating"}

			err := s.validateModelDependencies(context.Background(), appManifest, task)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateModelDependencies() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("validateModelDependencies() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("validateModelDependencies() error = %q, want substring %q", err.Error(), tt.wantErr)
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

func TestValidateModelDependenciesTaskOptional(t *testing.T) {
	s := newValidationServer(nil, true)
	appManifest := &manifest.AppManifest{Spec: manifest.Spec{Models: map[string]manifest.ModelMapping{
		"clip": {ID: "clip_vit_b_32"},
	}}}

	// nil task (sync InstallApp path): warning must log, not panic.
	if err := s.validateModelDependencies(context.Background(), appManifest, nil); err != nil {
		t.Fatalf("validateModelDependencies(nil task) unexpected error: %v", err)
	}

	// Attached task: progress message replaced with the warning.
	task := &InstallTask{ID: "t", Phase: "validating", Message: "Validation complete"}
	if err := s.validateModelDependencies(context.Background(), appManifest, task); err != nil {
		t.Fatalf("validateModelDependencies() unexpected error: %v", err)
	}
	_, _, message, _, _ := task.Snapshot()
	if !strings.HasPrefix(message, "Warning: ") {
		t.Errorf("task message = %q, want Warning: prefix", message)
	}
}
