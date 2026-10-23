package server

import (
	"reflect"
	"testing"
)

func TestRecoverySourceCandidates(t *testing.T) {
	tests := []struct {
		name            string
		requiredRef     string
		primary         bool
		archivedTags    []string
		importedPrimary string
		want            []string
	}{
		{
			name:            "unqualified Docker tag matches normalized manifest ref",
			requiredRef:     "docker.io/neoruntime/model-showcase:1.4.7",
			primary:         true,
			archivedTags:    []string{"neoruntime/model-showcase:1.4.7"},
			importedPrimary: "neoruntime/model-showcase:1.4.7",
			want: []string{
				"neoruntime/model-showcase:1.4.7",
				"docker.io/neoruntime/model-showcase:1.4.7",
			},
		},
		{
			name:            "primary preserves install-time retag behavior",
			requiredRef:     "docker.io/acme/renamed:2.0",
			primary:         true,
			archivedTags:    []string{"acme/original:1.0"},
			importedPrimary: "acme/original:1.0",
			want:            []string{"acme/original:1.0"},
		},
		{
			name:            "secondary cannot use an unrelated primary image",
			requiredRef:     "docker.io/acme/worker:2.0",
			primary:         false,
			archivedTags:    []string{"acme/main:2.0"},
			importedPrimary: "acme/main:2.0",
			want:            nil,
		},
		{
			name:            "secondary selects only its matching archived image",
			requiredRef:     "docker.io/acme/worker:2.0",
			primary:         false,
			archivedTags:    []string{"acme/main:2.0", "acme/worker:2.0"},
			importedPrimary: "acme/main:2.0",
			want: []string{
				"acme/worker:2.0",
				"docker.io/acme/worker:2.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recoverySourceCandidates(tt.requiredRef, tt.primary, tt.archivedTags, tt.importedPrimary)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("recoverySourceCandidates() = %v, want %v", got, tt.want)
			}
		})
	}
}
