package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aipc/platform/app-manager/containerd"
	"aipc/platform/app-manager/manifest"
	"aipc/platform/app-manager/registry"
	"aipc/platform/common/logger"
	"aipc/platform/common/utils"
)

// imageReferencesForApp applies the normalized image recorded at install time
// for legacy single-container apps, then returns every image the app needs.
func imageReferencesForApp(appInfo *registry.AppInfo, appManifest *manifest.AppManifest) []string {
	if !appManifest.IsMultiContainer() && appInfo != nil && appInfo.Image != "" {
		appManifest.Spec.Image = appInfo.Image
	}
	return appManifest.ImageReferences()
}

// recoverySourceCandidates returns only archive images that can legitimately
// satisfy requiredRef. The primary image retains the install-time behavior of
// binding the archive's first image to the manifest's primary reference; all
// other images must have a matching RepoTag so a multi-container app can never
// accidentally run its primary image in every container.
func recoverySourceCandidates(requiredRef string, primary bool, archivedTags []string, importedPrimary string) []string {
	seen := make(map[string]bool)
	var candidates []string
	add := func(ref string) {
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		candidates = append(candidates, ref)
	}

	for _, tag := range archivedTags {
		if manifest.NormalizeImageName(tag) != requiredRef {
			continue
		}
		add(tag)
		add(manifest.NormalizeImageName(tag))
	}
	if primary {
		add(importedPrimary)
	}
	return candidates
}

// unavailableImageReferences checks both the image index and its unpacked
// overlayfs state. An image record without usable snapshots is not startable.
func (s *AppManagerServer) unavailableImageReferences(ctx context.Context, refs []string) []string {
	var unavailable []string
	for _, ref := range refs {
		if _, err := s.client.GetImage(ctx, ref); err != nil {
			unavailable = append(unavailable, ref)
			continue
		}
		if err := s.client.EnsureImageUnpacked(ctx, ref); err != nil {
			logger.Warn("Image %s is indexed but not usable: %v", ref, err)
			unavailable = append(unavailable, ref)
		}
	}
	return unavailable
}

// restoreAppImagesFromSavedTar rebuilds containerd's volatile image index
// from the per-app archive retained on persistent storage. It also recreates
// normalized manifest tags, which are metadata and are lost with meta.db.
func (s *AppManagerServer) restoreAppImagesFromSavedTar(ctx context.Context, appID string, refs []string) error {
	if err := requireSafePathSegment("app id", appID); err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}

	tarPath := containerd.SavedImageTarPath(appID)
	if _, err := os.Stat(tarPath); err != nil {
		return fmt.Errorf("saved image archive %s is unavailable: %w", tarPath, err)
	}

	archivedTags := utils.ExtractAllImageNamesFromTar(tarPath)
	logger.Info("[IMAGE-RECOVERY] Re-importing app %s from %s (RepoTags: %v)", appID, tarPath, archivedTags)
	importedPrimary, err := s.client.ImportImage(ctx, tarPath, "")
	if err != nil {
		return fmt.Errorf("import saved image archive %s: %w", tarPath, err)
	}

	var failures []string
	for i, ref := range refs {
		needsRetag := false
		if _, getErr := s.client.GetImage(ctx, ref); getErr != nil {
			needsRetag = true
		} else if unpackErr := s.client.EnsureImageUnpacked(ctx, ref); unpackErr != nil {
			// The record survived but its target/snapshot state did not. Replace
			// the manifest tag from the freshly imported archive before retrying.
			logger.Warn("[IMAGE-RECOVERY] Existing image %s is unusable after import: %v", ref, unpackErr)
			needsRetag = true
		}
		if needsRetag {
			var tagErr error
			for _, source := range recoverySourceCandidates(ref, i == 0, archivedTags, importedPrimary) {
				if source == ref {
					continue
				}
				if _, sourceErr := s.client.GetImage(ctx, source); sourceErr != nil {
					continue
				}
				if tagErr = s.client.TagImage(ctx, source, ref); tagErr == nil {
					logger.Info("[IMAGE-RECOVERY] Restored manifest tag %s from %s", ref, source)
					break
				}
			}
			if _, verifyErr := s.client.GetImage(ctx, ref); verifyErr != nil {
				detail := verifyErr.Error()
				if tagErr != nil {
					detail = tagErr.Error()
				}
				failures = append(failures, fmt.Sprintf("%s (%s)", ref, detail))
				continue
			}
		}
		if unpackErr := s.client.EnsureImageUnpacked(ctx, ref); unpackErr != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", ref, unpackErr))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("saved archive did not restore required images: %s", strings.Join(failures, "; "))
	}
	logger.Info("[IMAGE-RECOVERY] App %s images restored from persistent archive", appID)
	return nil
}

// ensureAppImagesAvailable prefers the persistent local archive and only uses
// the registry as a fallback. This keeps imported/private apps startable on an
// offline device after an OS upgrade resets containerd metadata.
func (s *AppManagerServer) ensureAppImagesAvailable(ctx context.Context, appID string, refs []string) error {
	missing := s.unavailableImageReferences(ctx, refs)
	if len(missing) == 0 {
		return nil
	}

	logger.Warn("Images required by app %s are unavailable locally: %v", appID, missing)
	localErr := s.restoreAppImagesFromSavedTar(ctx, appID, refs)
	if localErr == nil {
		return nil
	}
	logger.Warn("[IMAGE-RECOVERY] Local recovery failed for app %s: %v", appID, localErr)

	missing = s.unavailableImageReferences(ctx, refs)
	for _, ref := range missing {
		logger.Warn("Image %s still unavailable after local recovery; attempting registry pull", ref)
		if pullErr := s.client.PullImage(ctx, ref); pullErr != nil {
			return fmt.Errorf("image %s is unavailable; local recovery failed: %v; registry pull failed: %w", ref, localErr, pullErr)
		}
	}

	if remaining := s.unavailableImageReferences(ctx, refs); len(remaining) > 0 {
		return fmt.Errorf("images remain unavailable after local recovery and registry pull: %v", remaining)
	}
	return nil
}

// restorePersistedAppImages proactively repairs every installed app after the
// snapshot check. This catches the OS-upgrade failure mode where containerd's
// image list is empty (and therefore contains no corrupt image to inspect).
func (s *AppManagerServer) restorePersistedAppImages() {
	if s.client == nil {
		return
	}
	ctx := s.withNamespace(context.Background())
	for _, appInfo := range s.registry.List() {
		unlockApp := s.lockAppMutation(appInfo.ID)
		if appInfo.ManifestPath == "" {
			unlockApp()
			continue
		}
		appManifest, err := manifest.LoadManifest(appInfo.ManifestPath)
		if err != nil {
			logger.Warn("[IMAGE-RECOVERY] Skip app %s: failed to load manifest: %v", appInfo.ID, err)
			unlockApp()
			continue
		}
		refs := imageReferencesForApp(appInfo, appManifest)
		missing := s.unavailableImageReferences(ctx, refs)
		if len(missing) == 0 {
			unlockApp()
			continue
		}
		logger.Warn("[IMAGE-RECOVERY] App %s is missing required images %v; restoring persistent archive", appInfo.ID, missing)
		if err := s.restoreAppImagesFromSavedTar(ctx, appInfo.ID, refs); err != nil {
			// Do not pull all installed apps during boot. StartApp retains the
			// online fallback and reports a precise error to its caller.
			logger.Error("[IMAGE-RECOVERY] Startup restore failed for app %s: %v", appInfo.ID, err)
		}
		unlockApp()
	}
}
