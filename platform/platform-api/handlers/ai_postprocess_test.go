package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	inferencepb "aipc/platform/ai-runtime/proto"
	"aipc/platform/common/constants"
	"aipc/platform/platform-api/model"
)

// newPostprocessTestEnv points the platform root at a temp dir (mirrors
// newPatchTestEnv) so materialized runtime copies land in a scratch models tree.
func newPostprocessTestEnv(t *testing.T) (string, func()) {
	t.Helper()
	oldRoot := constants.RootPath()
	root := t.TempDir()
	constants.SetRootPath(root)
	return root, func() { constants.SetRootPath(oldRoot) }
}

func writeHEF(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestDetectionPostprocessProfile(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{"empty config", "", model.DefaultDetectionProfile},
		{"valid s profile", `{"postprocess_profile":"hailo_yolov8s_384_640"}`, "hailo_yolov8s_384_640"},
		{"unknown profile", `{"postprocess_profile":"yolov8x_640_640"}`, model.DefaultDetectionProfile},
		{"invalid json", "{not json", model.DefaultDetectionProfile},
		{"wrong value type", `{"postprocess_profile":42}`, model.DefaultDetectionProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model.AIModel{Config: tt.cfg}
			if got := detectionPostprocessProfile(m); got != tt.want {
				t.Fatalf("detectionPostprocessProfile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectionVariantJSON(t *testing.T) {
	verbatim := `{"backend_function":"hailo_yolov8s","label_offset":0}`
	tests := []struct {
		name     string
		m        *model.AIModel
		want     map[string]interface{} // used unless passthru
		passthru bool                   // expect exact passthrough of m.Variant
	}{
		{
			name: "default profile composes backend function and tuning",
			m:    &model.AIModel{ModelType: "detection", Threshold: 0.4, MaxDetections: 32},
			want: map[string]interface{}{
				"backend_function":    "hailo_yolov8n",
				"detection_threshold": 0.4,
				"max_boxes":           float64(32),
			},
		},
		{
			name: "explicit s profile",
			m: &model.AIModel{
				ModelType: "detection",
				Config:    `{"postprocess_profile":"hailo_yolov8s_384_640"}`,
			},
			want: map[string]interface{}{"backend_function": "hailo_yolov8s"},
		},
		{
			name: "zero tuning values are omitted",
			m:    &model.AIModel{ModelType: "detection"},
			want: map[string]interface{}{"backend_function": "hailo_yolov8n"},
		},
		{
			name:     "json variant passes through verbatim",
			m:        &model.AIModel{ModelType: "detection", Variant: verbatim},
			passthru: true,
		},
		{
			name:     "non-detection variant unchanged",
			m:        &model.AIModel{ModelType: "classification", Variant: "resnet18"},
			passthru: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectionVariantJSON(tt.m)
			if tt.passthru {
				if got != tt.m.Variant {
					t.Fatalf("detectionVariantJSON() = %q, want passthrough %q", got, tt.m.Variant)
				}
				return
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("composed variant is not valid JSON: %v (%q)", err, got)
			}
			if !reflect.DeepEqual(parsed, tt.want) {
				t.Fatalf("detectionVariantJSON() = %v, want %v", parsed, tt.want)
			}
		})
	}
}

func TestRuntimeRegistration(t *testing.T) {
	root, restore := newPostprocessTestEnv(t)
	defer restore()

	blob := filepath.Join(root, "models", "blobs", "deadbeef.hef")
	writeHEF(t, blob, "blob-bytes")

	builtin := filepath.Join(root, "models", "detection", "hailo_yolov8n_384_640.hef")
	writeHEF(t, builtin, "builtin-bytes")

	t.Run("non-detection passes through", func(t *testing.T) {
		m := &model.AIModel{ModelID: "cls1", ModelType: "classification", FilePath: blob, Variant: "resnet18"}
		path, variant, err := runtimeRegistration(m)
		if err != nil {
			t.Fatalf("runtimeRegistration: %v", err)
		}
		if path != blob || variant != "resnet18" {
			t.Fatalf("got (%q, %q), want passthrough (%q, %q)", path, variant, blob, "resnet18")
		}
	})

	t.Run("builtin profile basename passes path through", func(t *testing.T) {
		m := &model.AIModel{ModelID: "det_builtin", ModelType: "detection", FilePath: builtin, Threshold: 0.3}
		path, variant, err := runtimeRegistration(m)
		if err != nil {
			t.Fatalf("runtimeRegistration: %v", err)
		}
		if path != builtin {
			t.Fatalf("path = %q, want builtin path %q unchanged", path, builtin)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(variant), &parsed); err != nil {
			t.Fatalf("variant not JSON: %v", err)
		}
		if parsed["backend_function"] != "hailo_yolov8n" {
			t.Fatalf("backend_function = %v, want hailo_yolov8n", parsed["backend_function"])
		}
	})

	t.Run("blob is materialized under profile name", func(t *testing.T) {
		m := &model.AIModel{
			ModelID: "fire_smoke", ModelType: "detection",
			FilePath: blob, Threshold: 0.4, MaxDetections: 32,
		}
		path, variant, err := runtimeRegistration(m)
		if err != nil {
			t.Fatalf("runtimeRegistration: %v", err)
		}
		want := filepath.Join(root, "models", "runtime", "fire_smoke", "hailo_yolov8n_384_640.hef")
		if path != want {
			t.Fatalf("path = %q, want %q", path, want)
		}
		srcStat, err := os.Stat(blob)
		if err != nil {
			t.Fatal(err)
		}
		dstStat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(srcStat, dstStat) {
			t.Fatal("runtime copy is not a link to the blob (different inodes)")
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(variant), &parsed); err != nil {
			t.Fatalf("variant not JSON: %v", err)
		}
		if parsed["detection_threshold"] != 0.4 || parsed["max_boxes"] != float64(32) {
			t.Fatalf("tuning not carried into variant: %v", parsed)
		}
	})

	t.Run("stored profile selects matching basename", func(t *testing.T) {
		m := &model.AIModel{
			ModelID: "fire_smoke", ModelType: "detection", FilePath: blob,
			Config: `{"postprocess_profile":"hailo_yolov8s_384_640"}`,
		}
		path, _, err := runtimeRegistration(m)
		if err != nil {
			t.Fatalf("runtimeRegistration: %v", err)
		}
		if !strings.HasSuffix(path, "hailo_yolov8s_384_640.hef") {
			t.Fatalf("path = %q, want hailo_yolov8s_384_640.hef suffix", path)
		}
	})

	t.Run("stale runtime copy is replaced", func(t *testing.T) {
		stale := filepath.Join(root, "models", "runtime", "stale_model", "hailo_yolov8n_384_640.hef")
		writeHEF(t, stale, "stale-bytes")
		m := &model.AIModel{ModelID: "stale_model", ModelType: "detection", FilePath: blob}
		path, _, err := runtimeRegistration(m)
		if err != nil {
			t.Fatalf("runtimeRegistration: %v", err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "blob-bytes" {
			t.Fatalf("runtime copy body = %q, want fresh %q", body, "blob-bytes")
		}
	})

	t.Run("unsafe model id rejected", func(t *testing.T) {
		for _, id := range []string{"../evil", "a/b", ".hidden", "a b", ""} {
			m := &model.AIModel{ModelID: id, ModelType: "detection", FilePath: blob}
			if _, _, err := runtimeRegistration(m); err == nil {
				t.Errorf("model id %q: expected error, got none", id)
			}
		}
	})
}

func TestRemoveRuntimeCopy(t *testing.T) {
	root, restore := newPostprocessTestEnv(t)
	defer restore()

	dir := filepath.Join(root, "models", "runtime", "fire_smoke")
	writeHEF(t, filepath.Join(dir, "hailo_yolov8n_384_640.hef"), "x")

	removeRuntimeCopy("fire_smoke")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("runtime dir still exists after removeRuntimeCopy (err=%v)", err)
	}

	// Unknown ids and ids unsafe as path segments must be no-ops, not panics.
	removeRuntimeCopy("never-existed")
	removeRuntimeCopy("../evil")
	if _, err := os.Stat(filepath.Join(root, "models")); err != nil {
		t.Fatalf("models root disturbed by unsafe-id call: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.hef")
	dst := filepath.Join(dir, "dst.hef")
	if err := os.WriteFile(src, []byte("src-body"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	body, err := os.ReadFile(dst)
	if err != nil || string(body) != "src-body" {
		t.Fatalf("copy mismatch: %q err=%v", body, err)
	}
	// Overwrite an existing dst with new content.
	if err := os.WriteFile(src, []byte("src-body-2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile overwrite: %v", err)
	}
	body, _ = os.ReadFile(dst)
	if string(body) != "src-body-2" {
		t.Fatalf("overwrite mismatch: %q", body)
	}
}

func TestZeroInputTensor(t *testing.T) {
	spec := &inferencepb.TensorSpec{Shape: []int32{1, 384, 640, 2}, Dtype: inferencepb.DataType_UINT8}
	tensor, err := zeroInputTensor(spec)
	if err != nil {
		t.Fatalf("zeroInputTensor: %v", err)
	}
	if want := 1 * 384 * 640 * 2; len(tensor.Data) != want {
		t.Fatalf("data len = %d, want %d", len(tensor.Data), want)
	}
	if tensor.Dtype != spec.Dtype || !reflect.DeepEqual(tensor.Shape, spec.Shape) {
		t.Fatalf("tensor spec not mirrored: %+v", tensor)
	}

	// NV12 inputs report an RGB-like shape (features=3) whose product is not
	// the host frame size — the runtime's byte_size is authoritative there.
	nv12 := &inferencepb.TensorSpec{
		Shape:     []int32{1, 384, 640, 3},
		Dtype:     inferencepb.DataType_UINT8,
		ByteSize:  384 * 640 * 3 / 2,
	}
	tensor, err = zeroInputTensor(nv12)
	if err != nil {
		t.Fatalf("zeroInputTensor nv12: %v", err)
	}
	if want := 384 * 640 * 3 / 2; len(tensor.Data) != want {
		t.Fatalf("nv12 data len = %d, want byte_size %d", len(tensor.Data), want)
	}

	floatSpec := &inferencepb.TensorSpec{Shape: []int32{1, 10}, Dtype: inferencepb.DataType_FLOAT32}
	tensor, err = zeroInputTensor(floatSpec)
	if err != nil {
		t.Fatalf("zeroInputTensor float: %v", err)
	}
	if want := 1 * 10 * 4; len(tensor.Data) != want {
		t.Fatalf("float data len = %d, want %d", len(tensor.Data), want)
	}

	bad := &inferencepb.TensorSpec{Shape: []int32{1, 0, 640}, Dtype: inferencepb.DataType_UINT8}
	if _, err := zeroInputTensor(bad); err == nil {
		t.Fatal("expected error for non-positive shape dim")
	}
}
