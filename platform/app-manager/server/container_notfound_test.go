package server

import (
	"errors"
	"fmt"
	"testing"

	"github.com/containerd/containerd/errdefs"
)

func TestStatusErrCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int32
	}{
		{"containerd not-found", fmt.Errorf("load container: %w", errdefs.ErrNotFound), 404},
		{"plain error", errors.New("rpc timeout"), 0},
		{"nil error", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusErrCode(tc.err); got != tc.want {
				t.Fatalf("statusErrCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
