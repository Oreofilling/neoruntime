package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/grpc"

	camerapb "aipc/platform/camera-daemon/proto"
)

// fakeStreamDaemon counts RemoveStream RPCs so tests can assert the handler
// short-circuits on unknown streams without touching the camera pipeline.
type fakeStreamDaemon struct {
	camerapb.UnimplementedCameraControlServer
	mu          sync.Mutex
	removeCalls int
}

func (f *fakeStreamDaemon) RemoveStream(ctx context.Context, req *camerapb.RemoveStreamRequest) (*camerapb.StreamOperationResponse, error) {
	f.mu.Lock()
	f.removeCalls++
	f.mu.Unlock()
	return &camerapb.StreamOperationResponse{Success: true}, nil
}

func (f *fakeStreamDaemon) removed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removeCalls
}

// writeStreamConfig writes a minimal camera-daemon YAML listing the given
// streams; readAllEncoderParams only needs the encoders entries.
func writeStreamConfig(t *testing.T, streams ...string) string {
	t.Helper()
	cfg := "pipeline:\n  sensor: imx662\nencoders:\n"
	for _, s := range streams {
		cfg += "  - stream_name: " + s + "\n    codec: h264\n    width: 1920\n    height: 1080\n    fps: 30\n    bitrate: 4096\n    gop: 30\n    enabled: true\n"
	}
	path := filepath.Join(t.TempDir(), "camera-daemon.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestRemoveStream_UnknownStream_Returns404WithoutPipelineCall asserts that
// deleting a stream absent from the YAML config is a clean 404 and never
// reaches the camera daemon (previously: bogus HAL request -> 500).
func TestRemoveStream_UnknownStream_Returns404WithoutPipelineCall(t *testing.T) {
	fake := &fakeStreamDaemon{}
	conn := newBufconnServer(t, func(srv *grpc.Server) {
		camerapb.RegisterCameraControlServer(srv, fake)
	})
	h := NewMediaHandlers(writeStreamConfig(t, "main", "sub"), conn, nil, nil)

	w := performRoute(t, http.MethodDelete, "/media/streams/:name", "/media/streams/__apitest__", h.RemoveStream)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s), want 404", w.Code, w.Body.String())
	}
	if code := decodeEnvelope(t, w); code != CodeNotFound {
		t.Fatalf("business code = %d, want %d", code, CodeNotFound)
	}
	if n := fake.removed(); n != 0 {
		t.Fatalf("RemoveStream RPC called %d time(s), want 0", n)
	}
}

// TestRemoveStream_MainStream_Rejected asserts the main-stream guard fires
// before the config lookup (and certainly before the daemon).
func TestRemoveStream_MainStream_Rejected(t *testing.T) {
	fake := &fakeStreamDaemon{}
	conn := newBufconnServer(t, func(srv *grpc.Server) {
		camerapb.RegisterCameraControlServer(srv, fake)
	})
	h := NewMediaHandlers(writeStreamConfig(t, "main"), conn, nil, nil)

	w := performRoute(t, http.MethodDelete, "/media/streams/:name", "/media/streams/main", h.RemoveStream)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if n := fake.removed(); n != 0 {
		t.Fatalf("RemoveStream RPC called %d time(s), want 0", n)
	}
}
