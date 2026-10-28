package containerd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImageTarAtPublishesAtomically(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "app.tar")
	if err := os.WriteFile(destination, []byte("old archive"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := saveImageTarAt(filepath.Join(dir, "missing.tar"), destination); err == nil {
		t.Fatal("saveImageTarAt unexpectedly accepted a missing source")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old archive" {
		t.Fatalf("failed publication damaged old archive: %q", got)
	}

	source := filepath.Join(dir, "new.tar")
	if err := os.WriteFile(source, []byte("new archive"), 0600); err != nil {
		t.Fatal(err)
	}
	staged, err := stageImageTarAt(source, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old archive" {
		t.Fatalf("staging replaced installed archive early: %q", got)
	}
	if err := publishStagedImageTarAt(staged, destination); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new archive" {
		t.Fatalf("published archive = %q, want new archive", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".image-recovery-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary recovery archives remain: %v", matches)
	}
}

func TestRemoveSavedImageTarAtIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.tar")
	if err := os.WriteFile(path, []byte("stale archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := removeSavedImageTarAt(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale archive remains: %v", err)
	}
	if err := removeSavedImageTarAt(path); err != nil {
		t.Fatalf("second removal was not idempotent: %v", err)
	}
}
