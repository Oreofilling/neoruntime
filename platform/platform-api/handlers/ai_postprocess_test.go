package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Registration-time guard tests. The load-time composition tests
// (variant composition, materialization, smoke-test tensor sizing) moved to
// platform/modelload/runtime_test.go along with the code they cover.

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

// completeVariantJSON is a schema-valid custom variant blob — the bar every
// user-supplied `{…}` variant must clear before it may be stored.
func completeVariantJSON(fn string) string {
	return `{"backend_function":"` + fn + `","iou_threshold":0.45,"detection_threshold":0.5,` +
		`"output_activation":"none","label_offset":1,"max_boxes":32,"labels":["fire","smoke"]}`
}

func TestValidateDetectionVariant(t *testing.T) {
	tests := []struct {
		name    string
		variant string
		wantErr string // empty means must pass
	}{
		{"empty variant", "", ""},
		{"plugin basename passes through", "hailo_yolov8s", ""},
		{"complete n function", completeVariantJSON("hailo_yolov8n"), ""},
		{"complete s function", completeVariantJSON("hailo_yolov8s"), ""},
		{"complete m function", completeVariantJSON("hailo_yolov8m"), ""},
		{
			"invalid json",
			`{"backend_function":`,
			"not valid JSON",
		},
		{
			"missing keys",
			`{"backend_function":"hailo_yolov8n","detection_threshold":0.5}`,
			"missing required key(s): iou_threshold, output_activation, label_offset, max_boxes, labels",
		},
		{
			"generic function rejected",
			completeVariantJSON("yolov8s"),
			`backend_function "yolov8s" is not supported`,
		},
		{
			"non-string function rejected",
			`{"backend_function":123,"iou_threshold":0.45,"detection_threshold":0.5,` +
				`"output_activation":"none","label_offset":1,"max_boxes":32,"labels":[]}`,
			"is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDetectionVariant(tt.variant)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDetectionVariant(%q) = %v, want nil", tt.variant, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateDetectionVariant(%q) = %v, want error containing %q", tt.variant, err, tt.wantErr)
			}
		})
	}
}
