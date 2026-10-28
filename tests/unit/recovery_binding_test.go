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
