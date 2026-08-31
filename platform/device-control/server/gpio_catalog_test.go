package main

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "aipc/platform/device-control/proto"
)

// newGPIOCatalogServer builds a server with the NE503 GPIO catalog and no
// camera-daemon link: readGPIOOnce then fails with "GPIO read not available",
// which distinguishes cleanly from the catalog rejection message.
func newGPIOCatalogServer() *DeviceControlServer {
	cfg := &Config{}
	cfg.Capabilities.GPIO.AvailablePins = []uint32{12, 13, 21, 22}
	cfg.Capabilities.GPIO.InputPins = []uint32{12, 13}
	cfg.Capabilities.GPIO.OutputPins = []uint32{21, 22}
	return &DeviceControlServer{config: cfg}
}

func TestGPIOPinKnown(t *testing.T) {
	available := []uint32{12, 13, 21, 22}
	for _, pin := range available {
		if !gpioPinKnown(available, pin) {
			t.Errorf("gpioPinKnown(%d) = false, want true", pin)
		}
	}
	for _, pin := range []uint32{0, 1, 19, 99, 255} {
		if gpioPinKnown(available, pin) {
			t.Errorf("gpioPinKnown(%d) = true, want false", pin)
		}
	}
}

// TestGPIORead_PinOutsideCatalog_RejectedBeforeMCU pins down the contract the
// platform-api 404 mapping relies on: unknown pins surface as codes.NotFound
// instead of being forwarded to the MCU as a raw pin byte.
func TestGPIORead_PinOutsideCatalog_RejectedBeforeMCU(t *testing.T) {
	s := newGPIOCatalogServer()

	_, err := s.GPIORead(context.Background(), &pb.GPIOReadRequest{Pin: 1})
	if err == nil {
		t.Fatal("GPIORead(pin=1) err = nil, want NotFound")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want %v", status.Code(err), codes.NotFound)
	}
	if !strings.Contains(err.Error(), "not in the GPIO catalog") {
		t.Fatalf("error %q missing catalog hint", err.Error())
	}
}

// TestGPIORead_CatalogPin_ReachesReader asserts known pins still flow through
// to the reader (which reports the missing MCU link in this harness).
func TestGPIORead_CatalogPin_ReachesReader(t *testing.T) {
	s := newGPIOCatalogServer()

	resp, err := s.GPIORead(context.Background(), &pb.GPIOReadRequest{Pin: 12})
	if err != nil {
		t.Fatalf("GPIORead(pin=12) err = %v, want nil", err)
	}
	if resp.Status == nil || resp.Status.Success {
		t.Fatalf("expected per-pin failure (no MCU link in test), got %+v", resp.Status)
	}
	if !strings.Contains(resp.Status.Message, "not available") {
		t.Fatalf("message %q: expected reader failure, not catalog rejection", resp.Status.Message)
	}
}

// TestGPIOBatchRead_OutOfCatalogPin_ReportedPerPinWithoutMCU asserts a batch
// containing an unknown pin reports that pin as failed while still reading
// the cataloged ones.
func TestGPIOBatchRead_OutOfCatalogPin_ReportedPerPinWithoutMCU(t *testing.T) {
	s := newGPIOCatalogServer()

	resp, err := s.GPIOBatchRead(context.Background(), &pb.GPIOBatchReadRequest{
		Pins: []uint32{12, 99},
	})
	if err != nil {
		t.Fatalf("GPIOBatchRead err = %v, want nil (per-pin statuses)", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}

	byPin := map[uint32]*pb.GPIOReadResponse{}
	for _, r := range resp.Results {
		byPin[r.Pin] = r
	}
	if r := byPin[99]; r == nil || r.Status == nil || r.Status.Success ||
		!strings.Contains(r.Status.Message, "not in the GPIO catalog") {
		t.Fatalf("pin 99 result = %+v, want failed with catalog message", r)
	}
	if r := byPin[12]; r == nil || r.Status == nil || r.Status.Success ||
		!strings.Contains(r.Status.Message, "not available") {
		t.Fatalf("pin 12 result = %+v, want reader failure (not catalog rejection)", r)
	}
	if byPin[12].Direction != "input" {
		t.Fatalf("pin 12 direction = %q, want input (from config)", byPin[12].Direction)
	}
}
