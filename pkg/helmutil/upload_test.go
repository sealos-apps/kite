package helmutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
)

func TestLoadRepositoryUploadConfigUsesOCIAndOfflineImageConfig(t *testing.T) {
	t.Setenv(ociRegistryBaseEnv, "oci://registry.local/kite-helm")
	t.Setenv(ociRepositoryNameEnv, "offline")
	t.Setenv(ociChartUploadMaxBytesEnv, "256MiB")
	t.Setenv(imageUploadMaxBytesEnv, "8GiB")
	t.Setenv(imageUploadRepositoryPrefixEnv, "library")
	originalRegistry := common.HelmOfflineImagesRegistry
	common.HelmOfflineImagesRegistry = "https://hub.local"
	t.Cleanup(func() {
		common.HelmOfflineImagesRegistry = originalRegistry
	})

	config, err := LoadRepositoryUploadConfig()
	require.NoError(t, err)
	require.True(t, config.Chart.Configured)
	require.Equal(t, "oci://registry.local/kite-helm", config.Chart.RegistryBase)
	require.Equal(t, "offline", config.Chart.RepositoryName)
	require.Equal(t, int64(256*1024*1024), config.Chart.MaxBytes)
	require.True(t, config.Image.Configured)
	require.Equal(t, "hub.local", config.Image.Registry)
	require.Equal(t, "library", config.Image.RepositoryPrefix)
	require.Equal(t, int64(8*1024*1024*1024), config.Image.MaxBytes)
}

func TestBuildImageUploadReferenceRejectsAbsoluteInput(t *testing.T) {
	config := imageUploadConfig{
		ContainerImageUploadConfig: ContainerImageUploadConfig{
			Registry:         "registry.local",
			RepositoryPrefix: "kite-images",
		},
	}

	for _, repository := range []string{
		"registry.other/app",
		"http://registry.local/app",
		"team/app:tag",
		"team/../app",
		"team/app@sha256:abc",
	} {
		t.Run(repository, func(t *testing.T) {
			_, err := buildImageUploadReference(config, repository, "1.0.0")
			require.Error(t, err)
			require.ErrorIs(t, err, ErrUploadValidation)
		})
	}
}

func TestBuildImageUploadReferenceUsesConfiguredPrefix(t *testing.T) {
	config := imageUploadConfig{
		ContainerImageUploadConfig: ContainerImageUploadConfig{
			Registry:         "registry.local:5000",
			RepositoryPrefix: "kite-images",
		},
		Options: OCIRegistryOptions{PlainHTTP: true},
	}

	ref, err := buildImageUploadReference(config, "apps/demo", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "registry.local:5000/kite-images/apps/demo:1.0.0", ref.Name())
}

func TestBuildImageUploadReferenceAllowsEmptyPrefix(t *testing.T) {
	config := imageUploadConfig{
		ContainerImageUploadConfig: ContainerImageUploadConfig{
			Registry: "registry.local:5000",
		},
		Options: OCIRegistryOptions{PlainHTTP: true},
	}

	ref, err := buildImageUploadReference(config, "apps/demo", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "registry.local:5000/apps/demo:1.0.0", ref.Name())
}

func TestParseByteSize(t *testing.T) {
	tests := map[string]int64{
		"42":    42,
		"5KB":   5000,
		"5KiB":  5120,
		"12mb":  12 * 1000 * 1000,
		"12MiB": 12 * 1024 * 1024,
		"2GiB":  2 * 1024 * 1024 * 1024,
		"2 GB":  2 * 1000 * 1000 * 1000,
	}
	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			got, err := parseByteSize(value)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	_, err := parseByteSize("bad")
	require.Error(t, err)
}

func TestSafeTarTargetPathRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	_, err := safeTarTargetPath(dest, "../oci-layout")
	require.Error(t, err)
	_, err = safeTarTargetPath(dest, "/absolute")
	require.Error(t, err)

	target, err := safeTarTargetPath(dest, "blobs/sha256/demo")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dest, "blobs", "sha256", "demo"), target)
}

func TestExtractTarToDirRejectsTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "layout.tar")
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	require.NoError(t, writer.WriteHeader(&tar.Header{
		Name: "../escape",
		Mode: 0o600,
		Size: 1,
	}))
	_, err := writer.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0o600))

	err = extractTarToDir(archivePath, t.TempDir(), int64(buf.Len()))
	require.Error(t, err)
}

func TestExtractTarToDirSupportsGzipArchives(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "layout.tar.gz")
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("layout")
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name: "oci-layout",
		Mode: 0o600,
		Size: int64(len(content)),
	}))
	_, err := tarWriter.Write(content)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0o600))

	destDir := t.TempDir()
	require.NoError(t, extractTarToDir(archivePath, destDir, int64(buf.Len())))
	got, err := os.ReadFile(filepath.Join(destDir, "oci-layout"))
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestExtractTarToDirRejectsExpandedSizeOverLimit(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "layout.tar")
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	content := []byte("layout")
	require.NoError(t, writer.WriteHeader(&tar.Header{
		Name: "oci-layout",
		Mode: 0o600,
		Size: int64(len(content)),
	}))
	_, err := writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0o600))

	err = extractTarToDir(archivePath, t.TempDir(), int64(len(content)-1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "extracted OCI layout exceeds")
}
