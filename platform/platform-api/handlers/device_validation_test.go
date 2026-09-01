package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Physical-write endpoints used to execute with Go zero values when the body
// omitted documented required fields (an empty body POSTed to /device/gpio
// wrote pin=0/value=false for real). Binding now rejects those bodies with
// 400 before any gRPC call is attempted, so a lazy (never-dialed) client conn
// is enough to drive the tests: a body that passes binding reaches the gRPC
// call and fails with CodeDeviceError -> 500, never 400 and never 200.
func newDeviceValidationRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := grpc.NewClient("passthrough:///unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("lazy grpc client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	h := NewAPIHandlers(&GRPCClients{DeviceControl: conn}, "", nil, nil,
		"", "", nil, nil, "", nil)
	r := gin.New()
	r.POST("/device/light", h.SetLight)
	r.POST("/device/ir-led", h.SetIrLed)
	r.POST("/device/ir-cut", h.SetIrCut)
	r.POST("/device/zoom", h.ControlZoom)
	r.POST("/device/focus", h.ControlFocus)
	r.POST("/device/autofocus", h.SetAutofocus)
	r.POST("/device/gpio", h.GPIOWrite)
	r.POST("/device/fan", h.SetFan)
	return r
}

func TestDeviceWritesRejectMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name       string
		route      string
		body       string
		wantStatus int // 400 = rejected at binding; 500 = passed binding, gRPC failed on the lazy conn
	}{
		// Missing / empty bodies must be rejected before any device write.
		{"light empty body", "/device/light", `{}`, http.StatusBadRequest},
		{"light level omitted", "/device/light", `{"other":1}`, http.StatusBadRequest},
		{"light level above 100", "/device/light", `{"level":101}`, http.StatusBadRequest},
		{"light level 0 is legal", "/device/light", `{"level":0}`, http.StatusInternalServerError},
		{"ir-led empty body", "/device/ir-led", `{}`, http.StatusBadRequest},
		{"ir-cut mode omitted", "/device/ir-cut", `{}`, http.StatusBadRequest},
		{"zoom empty body", "/device/zoom", `{}`, http.StatusBadRequest},
		{"zoom speed 0 rejected", "/device/zoom", `{"speed":0}`, http.StatusBadRequest},
		{"zoom speed above 100", "/device/zoom", `{"speed":150}`, http.StatusBadRequest},
		{"zoom speed below -100", "/device/zoom", `{"speed":-150}`, http.StatusBadRequest},
		{"focus empty body", "/device/focus", `{}`, http.StatusBadRequest},
		{"autofocus enable omitted", "/device/autofocus", `{}`, http.StatusBadRequest},
		{"autofocus enable false is legal", "/device/autofocus", `{"enable":false}`, http.StatusInternalServerError},
		{"gpio empty body", "/device/gpio", `{}`, http.StatusBadRequest},
		{"gpio value omitted", "/device/gpio", `{"pin":3}`, http.StatusBadRequest},
		{"gpio pin 0 and false are legal", "/device/gpio", `{"pin":0,"value":false}`, http.StatusInternalServerError},
		{"fan enable omitted", "/device/fan", `{"other":true}`, http.StatusBadRequest},
		{"fan enable true passes binding", "/device/fan", `{"enable":true}`, http.StatusInternalServerError},
	}

	r := newDeviceValidationRouter(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.route,
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(resp, req)
			if resp.Code != tc.wantStatus {
				t.Fatalf("body %s: got %d (%s), want %d",
					tc.body, resp.Code, resp.Body.String(), tc.wantStatus)
			}
		})
	}
}
