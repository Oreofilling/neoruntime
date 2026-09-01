package main

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"

	camerapb "aipc/platform/camera-daemon/proto"
	pb "aipc/platform/device-control/proto"
)

// mcuCallRecorder is a camerapb.CameraControlClient that counts (and would
// satisfy) every McuRawRequest. GPIO and PTZ handlers must leave calls at
// zero: the IDs they historically sent (0x30/0x31, 0x20-0x24) collide with
// real MCU commands (AIN_GET/RESET_SOC, LED_SET/LED_GET/IRCUT_SET/IRCUT_GET/
// PD_GET — host_link_proto.h), so issuing any of them drives unrelated
// hardware. See issue #46.
type mcuCallRecorder struct {
	camerapb.CameraControlClient // embed: other methods panic if ever used
	calls                        int
}

func (m *mcuCallRecorder) McuRawRequest(ctx context.Context, in *camerapb.McuRawRequestMessage,
	opts ...grpc.CallOption) (*camerapb.McuRawResponseMessage, error) {
	m.calls++
	return &camerapb.McuRawResponseMessage{Success: true, Payload: []byte{1}}, nil
}

// newCmdGuardServer builds a server whose GPIO catalog and PTZ flags are all
// open (so no handler can hide behind a config gate) and whose camera-daemon
// link records MCU traffic.
func newCmdGuardServer() (*DeviceControlServer, *mcuCallRecorder) {
	cfg := &Config{}
	cfg.Capabilities.GPIO.AvailablePins = []uint32{12, 13, 21, 22}
	cfg.Capabilities.GPIO.InputPins = []uint32{12, 13}
	cfg.Capabilities.GPIO.OutputPins = []uint32{21, 22}
	cfg.Capabilities.PTZ.Enabled = true
	cfg.Capabilities.PTZ.Presets = 16
	rec := &mcuCallRecorder{}
	s := &DeviceControlServer{config: cfg, cameraDaemonClient: rec}
	return s, rec
}

func assertNoMcuTraffic(t *testing.T, rec *mcuCallRecorder, op string) {
	t.Helper()
	if rec.calls != 0 {
		t.Fatalf("%s issued %d McuRawRequest(s), want 0 (issue #46: fabricated command IDs)", op, rec.calls)
	}
}

// TestGPIOOps_NeverTouchMCU is the issue #46 regression pin: cataloged or
// not, GPIO reads and writes must not send anything to the MCU — 0x31 is
// RESET_SOC and power-cycles the SoC.
func TestGPIOOps_NeverTouchMCU(t *testing.T) {
	s, rec := newCmdGuardServer()
	ctx := context.Background()

	readResp, err := s.GPIORead(ctx, &pb.GPIOReadRequest{Pin: 12})
	if err != nil {
		t.Fatalf("GPIORead err = %v, want nil (per-pin status)", err)
	}
	if readResp.Status == nil || readResp.Status.Success {
		t.Fatalf("GPIORead status = %+v, want unsupported failure", readResp.Status)
	}
	if !strings.Contains(readResp.Status.Message, "issue #46") {
		t.Fatalf("message %q: want it to cite the root cause", readResp.Status.Message)
	}

	batchResp, err := s.GPIOBatchRead(ctx, &pb.GPIOBatchReadRequest{})
	if err != nil {
		t.Fatalf("GPIOBatchRead err = %v, want nil", err)
	}
	if len(batchResp.Results) != len(batchResp.AvailablePins) {
		t.Fatalf("batch results = %d, want one per catalog pin (%d)",
			len(batchResp.Results), len(batchResp.AvailablePins))
	}
	for _, r := range batchResp.Results {
		if r.Status == nil || r.Status.Success {
			t.Fatalf("pin %d status = %+v, want unsupported failure", r.Pin, r.Status)
		}
	}

	writeResp, err := s.GPIOWrite(ctx, &pb.GPIOWriteRequest{Pin: 21, Value: true})
	if err != nil {
		t.Fatalf("GPIOWrite(catalog pin) err = %v, want nil (status-carried failure)", err)
	}
	if writeResp.Success {
		t.Fatal("GPIOWrite(catalog pin) = success, want unsupported failure")
	}

	_, err = s.GPIOWrite(ctx, &pb.GPIOWriteRequest{Pin: 99, Value: true})
	if err == nil {
		t.Fatal("GPIOWrite(pin=99) err = nil, want NotFound")
	}

	assertNoMcuTraffic(t, rec, "GPIO read/batch/write")
}

// TestPTZOps_NeverTouchMCU pins the sibling defect: with PTZ enabled in
// config, pan/tilt/stop/preset ops must still not send 0x20-0x24 — those IDs
// are LED_SET/LED_GET/IRCUT_SET/IRCUT_GET/PD_GET on this platform's MCU.
func TestPTZOps_NeverTouchMCU(t *testing.T) {
	s, rec := newCmdGuardServer()
	ctx := context.Background()

	ops := []struct {
		name string
		call func() (*pb.Status, error)
	}{
		{"Pan", func() (*pb.Status, error) {
			return s.Pan(ctx, &pb.PanRequest{Direction: pb.PanDirection_PAN_LEFT, Speed: 50})
		}},
		{"Tilt", func() (*pb.Status, error) {
			return s.Tilt(ctx, &pb.TiltRequest{Direction: pb.TiltDirection_TILT_UP, Speed: 50})
		}},
		{"PTZStop", func() (*pb.Status, error) {
			return s.PTZStop(ctx, &pb.PTZStopRequest{})
		}},
		{"SavePreset", func() (*pb.Status, error) {
			return s.SavePreset(ctx, &pb.PresetRequest{PresetId: 1})
		}},
		{"CallPreset", func() (*pb.Status, error) {
			return s.CallPreset(ctx, &pb.PresetRequest{PresetId: 1})
		}},
	}

	for _, op := range ops {
		resp, err := op.call()
		if err != nil {
			t.Fatalf("%s err = %v, want nil (status-carried failure)", op.name, err)
		}
		if resp == nil || resp.Success {
			t.Fatalf("%s status = %+v, want unsupported failure", op.name, resp)
		}
	}

	assertNoMcuTraffic(t, rec, "PTZ pan/tilt/stop/save/call")
}
