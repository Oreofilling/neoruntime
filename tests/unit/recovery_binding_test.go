package unit

import (
	"os"
	"strings"
	"testing"
)

// The wtmp merge must never run once /etc/fstab holds the bind entry: the
// firstboot unit is ordered After=local-fs.target, so from the following
// boot on /var/log/wtmp IS the bind-mounted /data copy (same inode), and
// appending the file to itself with BusyBox cat never reaches EOF — it
// fills /data and blocks every After= service under TimeoutStartSec=0.
func TestFirstbootWtmpMergeSkipsBindMountedTarget(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/aipc-firstboot.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	guard := `[[ -s "$ut_dst" ]] && [[ ! "$ut_dst" -ef "$ut_src" ]]`
	guardAt := strings.Index(script, guard)
	mergeAt := strings.Index(script, `cat "$ut_dst" >>"$ut_src"`)
	if guardAt < 0 {
		t.Fatal("wtmp merge condition lacks the same-inode (-ef) guard for the bind-mounted target")
	}
	if mergeAt < 0 || guardAt > mergeAt {
		t.Fatal("the same-inode guard must be part of the merge condition that precedes the append")
	}
}

// The old recovery archive must be invalidated before the registry commit,
// not after it. Committing the registry first leaves a crash window where a
// self-consistent old archive (old tar + still-matching digest sidecar)
// sits behind a registry that already describes new content under the same
// mutable reference — after a metadata reset that pair verifies as valid
// and silently restores the old binary. (The registry.Update at the top of
// InstallApp only flips the old app's state and is not a content commit.)
func TestArchiveBindingInvalidatedBeforeRegistryCommit(t *testing.T) {
	raw, err := os.ReadFile("../../platform/app-manager/server/server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	commitAnchors := map[string]string{
		"InstallApp(":              "// Register or update app",
		"runAsyncInstallSnapshot(": `task.Update("registering", 90, "Saving app registry...")`,
	}
	for fn, anchor := range commitAnchors {
		start := strings.Index(source, "func (s *AppManagerServer) "+fn)
		if start < 0 {
			t.Fatalf("could not find %s in server.go", fn)
		}
		end := strings.Index(source[start+1:], "\nfunc ")
		if end < 0 {
			t.Fatalf("could not extract the body of %s", fn)
		}
		body := source[start : start+1+end]
		invalidate := strings.Index(body, "containerd.RemoveSavedImageTar")
		commit := strings.Index(body, anchor)
		if invalidate < 0 || commit < 0 {
			t.Fatalf("%s must both invalidate the previous archive and commit the registry", fn)
		}
		if invalidate > commit {
			t.Fatalf("%s invalidates the previous recovery archive after committing the registry; a crash in between resurrects stale content", fn)
		}
	}
}

// The digest-mismatch branch of restoreAppImagesFromSavedTar must purge the
// references the import just created before returning the rejection. Deleting
// the tar alone leaves the stale images resolving as available in containerd,
// so ensureAppImagesAvailable would recompute the missing set as empty and
// return success — the rejected content would keep running.
func TestRecoveryMismatchPurgesRejectedReferences(t *testing.T) {
	raw, err := os.ReadFile("../../platform/app-manager/server/image_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	purge := strings.Index(source, "s.client.RemoveImage(ctx, rejected, true)")
	reject := strings.Index(source, "does not match the binding recorded at publication")
	if purge < 0 || reject < 0 {
		t.Fatal("digest-mismatch branch must purge the imported references before rejecting")
	}
	if purge > reject {
		t.Fatal("rejected references must be purged before the mismatch error returns")
	}
}

// deploy.sh must confirm containerd is actually inactive before migrating its
// store: the bounded-systemctl wrapper only kills the waiting systemctl
// client, so a stop timeout (124) swallowed by `|| true` leaves the daemon
// running while the migration copies and deletes underneath it.
func TestDeployVerifiesContainerdStoppedBeforeMigration(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/deploy.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	guardAt := strings.Index(script, `inactive|failed|unknown) containerd_stopped=1; break ;;`)
	migrateAt := strings.Index(script, `migrate_persistent_dir "$CONTAINERD_OLD_ROOT"`)
	if guardAt < 0 || migrateAt < 0 {
		t.Fatal("deploy.sh must verify containerd is inactive before migrating its store")
	}
	if guardAt > migrateAt {
		t.Fatal("the terminal-state containerd guard must run before migrate_persistent_dir")
	}
	if !strings.Contains(script, "refusing to migrate its store") {
		t.Fatal("deploy.sh must fail loudly when containerd stays active after stop")
	}
}

// firstboot must confirm containerd reached an explicit terminal state before
// migrating its store, and must do so even when the activity probe above
// reported inactive: under the 60s systemctl wrapper a hung probe exits 124,
// which `is-active --quiet` callers read as "not active", and the swallowed
// stop timeout does not mean the stop job finished. Migration under a live
// daemon corrupts the store it is copying.
func TestFirstbootVerifiesContainerdStoppedBeforeMigration(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/aipc-firstboot.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	guardAt := strings.Index(script, `inactive|failed|unknown) stop_confirmed=1; break ;;`)
	migrateAt := strings.Index(script, `migrate_containerd_root "$current_root"`)
	if guardAt < 0 || migrateAt < 0 {
		t.Fatal("firstboot must confirm containerd stopped before migrating its store")
	}
	if guardAt > migrateAt {
		t.Fatal("the terminal-state guard must run before the containerd migration passes")
	}
	if !strings.Contains(script, "refusing to migrate its store") {
		t.Fatal("firstboot must refuse migration when containerd stays active")
	}
}

// The time saver's DefaultDependencies=no unit must order itself against
// umount.target: with only the shutdown targets in Before=, the unit runs as
// an unordered sibling of the unmounts and /data can be gone by ExecStart.
func TestTimeSaveUnitOrderedAgainstUmountTarget(t *testing.T) {
	raw, err := os.ReadFile("../../systemd/aipc-time-save.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)
	if !strings.Contains(unit, "Before=shutdown.target reboot.target halt.target umount.target") {
		t.Fatal("aipc-time-save.service must run Before=umount.target or /data may unmount first")
	}
	if !strings.Contains(unit, "RequiresMountsFor=/data") {
		t.Fatal("aipc-time-save.service must keep the /data mount alive until it finishes")
	}
}
