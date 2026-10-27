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

func TestSameImageReferences(t *testing.T) {
	if !sameImageReferences(
		[]string{"docker.io/acme/main:2", "docker.io/acme/worker:2"},
		[]string{"docker.io/acme/main:2", "docker.io/acme/worker:2"},
	) {
		t.Fatal("identical image references were treated as changed")
	}
	for _, changed := range [][]string{
		{"docker.io/acme/main:3", "docker.io/acme/worker:2"},
		{"docker.io/acme/worker:2", "docker.io/acme/main:2"},
		{"docker.io/acme/main:2"},
	} {
		if sameImageReferences([]string{"docker.io/acme/main:2", "docker.io/acme/worker:2"}, changed) {
			t.Fatalf("changed image references were treated as identical: %v", changed)
		}
	}
}

func TestShouldDiscardSavedImageArchive(t *testing.T) {
	oldRefs := []string{"docker.io/acme/main:2"}
	tests := []struct {
		name          string
		isUpdate      bool
		hasNewArchive bool
		oldRefsKnown  bool
		newRefs       []string
		want          bool
	}{
		{name: "no-upload changed image", isUpdate: true, oldRefsKnown: true, newRefs: []string{"docker.io/acme/main:3"}, want: true},
		{name: "no-upload unchanged image", isUpdate: true, oldRefsKnown: true, newRefs: oldRefs, want: false},
		{name: "unverifiable old binding", isUpdate: true, oldRefsKnown: false, newRefs: oldRefs, want: true},
		{name: "new local archive replaces old binding", isUpdate: true, hasNewArchive: true, oldRefsKnown: false, newRefs: []string{"docker.io/acme/main:3"}, want: false},
		{name: "fresh install has no stale binding", isUpdate: false, oldRefsKnown: false, newRefs: []string{"docker.io/acme/main:3"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldDiscardSavedImageArchive(tt.isUpdate, tt.hasNewArchive, tt.oldRefsKnown, oldRefs, tt.newRefs)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
