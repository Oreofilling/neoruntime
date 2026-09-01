package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildLegacyTar writes a docker-save legacy layout archive: root
// manifest.json + <config-sha256>.json + top-level layer.tar, with
// manifest.Layers referencing archive member paths ("layer.tar").
func buildLegacyTar(t *testing.T, dir, name string, layerContent []byte) string {
	t.Helper()
	config := map[string]any{
		"architecture": "arm64",
		"os":           "linux",
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": []string{"sha256:" + hex.EncodeToString(sha256Sum(layerContent))},
		},
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configName := hex.EncodeToString(sha256Sum(configBytes)) + ".json"

	manifest := []map[string]any{{
		"Config":   configName,
		"RepoTags": []string{"e2e/test-image:0.1.0"},
		"Layers":   []string{"layer.tar"},
	}}

	return writeTar(t, dir, name, func(tw *tar.Writer) {
		writeTarMember(t, tw, "layer.tar", layerContent)
		writeTarMember(t, tw, configName, configBytes)
		writeTarMember(t, tw, "manifest.json", mustJSON(t, manifest))
	})
}

// buildDigestRefTar writes the OCI-style archive that the containerd
// importer rejects: Layers reference digests instead of member paths.
func buildDigestRefTar(t *testing.T, dir, name string, layerContent []byte) string {
	t.Helper()
	digest := "sha256:" + hex.EncodeToString(sha256Sum(layerContent))
	manifest := []map[string]any{{
		"Config":   digest + ".json",
		"RepoTags": []string{"e2e/test-image:0.1.0"},
		"Layers":   []string{digest},
	}}
	return writeTar(t, dir, name, func(tw *tar.Writer) {
		writeTarMember(t, tw, digest+"/layer.tar", layerContent)
		writeTarMember(t, tw, "manifest.json", mustJSON(t, manifest))
	})
}

func buildNoManifestTar(t *testing.T, dir, name string) string {
	t.Helper()
	return writeTar(t, dir, name, func(tw *tar.Writer) {
		writeTarMember(t, tw, "README", []byte("not an image"))
	})
}

func buildTruncatedTar(t *testing.T, dir, name string) string {
	t.Helper()
	full := buildLegacyTar(t, dir, name+".full", []byte("payload"))
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read full tar: %v", err)
	}
	// Cut in the middle: keep just enough to look like a tar start.
	truncated := data[:len(data)/2]
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, truncated, 0644); err != nil {
		t.Fatalf("write truncated tar: %v", err)
	}
	return path
}

func gzipFile(t *testing.T, src, dst string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(dst, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	return dst
}

func writeTar(t *testing.T, dir, name string, fill func(*tar.Writer)) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	fill(tw)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}
	return path
}

func writeTarMember(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("write header %s: %v", name, err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write member %s: %v", name, err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func TestValidateDockerSaveTar_LegacyLayoutPasses(t *testing.T) {
	dir := t.TempDir()
	path := buildLegacyTar(t, dir, "valid.tar", []byte("layer-bytes"))
	if err := ValidateDockerSaveTar(path); err != nil {
		t.Fatalf("legacy layout should pass, got: %v", err)
	}
}

func TestValidateDockerSaveTar_DigestRefRejected(t *testing.T) {
	dir := t.TempDir()
	path := buildDigestRefTar(t, dir, "digest.tar", []byte("layer-bytes"))
	err := ValidateDockerSaveTar(path)
	if err == nil {
		t.Fatal("digest-reference Layers must be rejected")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should name the missing member, got: %v", err)
	}
}

func TestValidateDockerSaveTar_MissingManifestRejected(t *testing.T) {
	dir := t.TempDir()
	path := buildNoManifestTar(t, dir, "nomanifest.tar")
	err := ValidateDockerSaveTar(path)
	if err == nil {
		t.Fatal("archive without manifest.json must be rejected")
	}
	if !strings.Contains(err.Error(), "manifest.json") {
		t.Errorf("error should mention manifest.json, got: %v", err)
	}
}

func TestValidateDockerSaveTar_TruncatedRejected(t *testing.T) {
	dir := t.TempDir()
	path := buildTruncatedTar(t, dir, "truncated.tar")
	if err := ValidateDockerSaveTar(path); err == nil {
		t.Fatal("truncated tar must be rejected")
	}
}

func TestValidateDockerSaveTar_GzipLegacyPasses(t *testing.T) {
	dir := t.TempDir()
	plain := buildLegacyTar(t, dir, "plain.tar", []byte("layer-bytes"))
	path := gzipFile(t, plain, filepath.Join(dir, "valid.tar.gz"))
	if err := ValidateDockerSaveTar(path); err != nil {
		t.Fatalf("gzipped legacy layout should pass, got: %v", err)
	}
}

func TestValidateDockerSaveTar_EmptyRepoTagsPass(t *testing.T) {
	dir := t.TempDir()
	layer := []byte("layer-bytes")
	config := map[string]any{"architecture": "arm64", "os": "linux"}
	configBytes := mustJSON(t, config)
	configName := hex.EncodeToString(sha256Sum(configBytes)) + ".json"
	manifest := []map[string]any{{
		"Config":   configName,
		"RepoTags": nil,
		"Layers":   []string{"layer.tar"},
	}}
	path := writeTar(t, dir, "untagged.tar", func(tw *tar.Writer) {
		writeTarMember(t, tw, "layer.tar", layer)
		writeTarMember(t, tw, configName, configBytes)
		writeTarMember(t, tw, "manifest.json", mustJSON(t, manifest))
	})
	if err := ValidateDockerSaveTar(path); err != nil {
		t.Fatalf("untagged (empty RepoTags) image should pass, got: %v", err)
	}
}

func TestExtractImageNameFromTar_PlainAndGzip(t *testing.T) {
	dir := t.TempDir()
	plain := buildLegacyTar(t, dir, "plain.tar", []byte("layer-bytes"))

	if got := ExtractImageNameFromTar(plain); got != "e2e/test-image:0.1.0" {
		t.Errorf("plain tar image name = %q, want e2e/test-image:0.1.0", got)
	}

	gz := gzipFile(t, plain, filepath.Join(dir, "image.tar.gz"))
	if got := ExtractImageNameFromTar(gz); got != "e2e/test-image:0.1.0" {
		t.Errorf("gzip tar image name = %q, want e2e/test-image:0.1.0", got)
	}
}
