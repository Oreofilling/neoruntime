package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"aipc/platform/common/constants"
	"aipc/platform/common/logger"
)

// safeAppIDPattern bounds app IDs used as directory names under the
// managed manifests root. manifest.Validate only rejects empty IDs, so
// this is the only thing stopping an ID like "../bin" from turning
// into a path.
var safeAppIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func managedManifestsRoot() string {
	// Derived from the shared root rather than Apps.ManifestsPath config:
	// that field is declared but unused on this side, and its value on
	// devices (/etc/aipc/apps) does not match where manifests actually
	// live. platform-api owns the real convention: <root>/apps/manifests.
	return filepath.Join(constants.RootPath(), "apps", "manifests")
}

// managedManifestDir reports the directory that owns manifestPath, but
// only when it is exactly <root>/apps/manifests/<appID> — a directory
// the platform created for this specific app. Anything else returns
// false: the manifests root itself, the install root, /tmp, another
// app's directory, or a path forged with separators in the app ID.
func managedManifestDir(manifestPath, appID string) (string, bool) {
	if manifestPath == "" || appID == "" || !safeAppIDPattern.MatchString(appID) {
		return "", false
	}
	dir := filepath.Dir(filepath.Clean(manifestPath))
	if dir != filepath.Join(managedManifestsRoot(), appID) {
		return "", false
	}
	return dir, true
}

// canonicalizeManifest copies the manifest file into the managed
// manifests root as <root>/apps/manifests/<appID>/app.yaml and returns
// that path. Callers arrive with arbitrary paths: platform-api uploads
// already land in the root (the copy is skipped), but CLI installs
// point at unpacked tarballs in /tmp and legacy installs at the
// install-root top level — paths whose parent directories uninstall
// cleanup must never act on, so the registry records the canonical
// copy instead. Source bytes are copied verbatim (comments and
// unknown fields survive); the caller-owned source file is left in
// place.
func canonicalizeManifest(manifestPath, appID string) (string, error) {
	if !safeAppIDPattern.MatchString(appID) {
		return "", fmt.Errorf("app id %q is not usable as a manifest directory name", appID)
	}
	canonical := filepath.Join(managedManifestsRoot(), appID, "app.yaml")
	if filepath.Clean(manifestPath) == canonical {
		return canonical, nil
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest %s: %w", manifestPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(canonical), 0755); err != nil {
		return "", fmt.Errorf("failed to create manifest dir %s: %w", filepath.Dir(canonical), err)
	}
	if err := os.WriteFile(canonical, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write canonical manifest %s: %w", canonical, err)
	}
	logger.Info("Manifest canonicalized: %s -> %s", manifestPath, canonical)
	return canonical, nil
}
