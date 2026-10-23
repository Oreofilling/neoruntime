package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSVerifyDoesNotPullRuntimeBeforeCondition(t *testing.T) {
	raw, err := os.ReadFile("../../systemd/aipc-os-verify.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)

	for _, forbidden := range []string{
		"Wants=aipc-autostart.service",
		"Wants=aipc-platform.target",
		"Requires=aipc-autostart.service",
		"Requires=aipc-platform.target",
	} {
		if strings.Contains(unit, forbidden) {
			t.Fatalf("verify unit unconditionally pulls the runtime through %q", forbidden)
		}
	}
	if !strings.Contains(unit, "ExecCondition=/usr/libexec/aipc-os-updater needs-verify") {
		t.Fatal("verify unit is missing the upgrade-state ExecCondition")
	}
	if !strings.Contains(unit, "ExecStartPre=/usr/bin/systemctl restart aipc-autostart.service") {
		t.Fatal("verify unit does not restore the runtime for a real verify boot")
	}
}

func TestFirstbootDoesNotEnableRuntimeServices(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/aipc-firstboot.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "systemctl enable aipc-healthmon.service") {
		t.Fatal("firstboot must not override aipc-cli system disable")
	}
}

func TestFirstbootContainerdMigrationPreservesPersistentMetadata(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/aipc-firstboot.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	start := strings.Index(script, "migrate_containerd_root() {")
	if start < 0 {
		t.Fatal("could not find migrate_containerd_root in firstboot script")
	}
	endMarker := "\n}\n\nconfigure_containerd_root() {"
	end := strings.Index(script[start:], endMarker)
	if end < 0 {
		t.Fatal("could not extract migrate_containerd_root from firstboot script")
	}
	function := script[start : start+end+len("\n}")]

	t.Run("persistent destination wins over fresh rootfs", func(t *testing.T) {
		tmp := t.TempDir()
		source := filepath.Join(tmp, "rootfs-containerd")
		destination := filepath.Join(tmp, "persistent-containerd")
		for _, dir := range []string{source, destination} {
			if err := os.MkdirAll(filepath.Join(dir, "io.containerd.metadata.v1.bolt"), 0755); err != nil {
				t.Fatal(err)
			}
		}
		sourceMeta := filepath.Join(source, "io.containerd.metadata.v1.bolt", "meta.db")
		destinationMeta := filepath.Join(destination, "io.containerd.metadata.v1.bolt", "meta.db")
		if err := os.WriteFile(sourceMeta, []byte("fresh-empty-index"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destinationMeta, []byte("persistent-image-index"), 0644); err != nil {
			t.Fatal(err)
		}

		runFirstbootMigration(t, function, source, destination)

		got, err := os.ReadFile(destinationMeta)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "persistent-image-index" {
			t.Fatalf("persistent meta.db was overwritten: got %q", got)
		}
		assertSymlinkTarget(t, source, destination)
	})

	t.Run("empty destination is seeded", func(t *testing.T) {
		tmp := t.TempDir()
		source := filepath.Join(tmp, "old-containerd")
		destination := filepath.Join(tmp, "persistent-containerd")
		if err := os.MkdirAll(source, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "seed"), []byte("old-state"), 0644); err != nil {
			t.Fatal(err)
		}

		runFirstbootMigration(t, function, source, destination)

		got, err := os.ReadFile(filepath.Join(destination, "seed"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "old-state" {
			t.Fatalf("destination was not seeded: got %q", got)
		}
		assertSymlinkTarget(t, source, destination)
	})
}

func runFirstbootMigration(t *testing.T, function, source, destination string) {
	t.Helper()
	command := "set -euo pipefail\nwarn() { :; }\nlog() { :; }\n" + function + "\nmigrate_containerd_root \"$1\" \"$2\""
	cmd := exec.Command("bash", "-c", command, "migration-test", source, destination)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("migration failed: %v\n%s", err, output)
	}
}

func assertSymlinkTarget(t *testing.T, link, want string) {
	t.Helper()
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("symlink target = %q, want %q", got, want)
	}
}

func TestCurrentRootInstallerPreservesRuntimeDisable(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/aipc-install-current-root.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)

	if !strings.Contains(script, `unit_enable_state["$name"]="$("$SYSTEMCTL" is-enabled "$name"`) {
		t.Fatal("current-root installer does not snapshot existing unit enable state")
	}
	if !strings.Contains(script, `disabled|masked|masked-runtime)`) {
		t.Fatal("current-root installer does not suppress explicitly disabled boot units")
	}
	if strings.Contains(script, "        aipc-platform.target\n") {
		t.Fatal("current-root installer must not explicitly start aipc-platform.target")
	}
}
