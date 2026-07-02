package helmutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
)

func TestBundleImageKeyForRegistryStripsConfiguredRegistry(t *testing.T) {
	key, err := bundleImageKeyForRegistry("registry.local/bitnami/nginx:1.27.0", "https://registry.local")
	require.NoError(t, err)
	require.Equal(t, "bitnami/nginx:1.27.0", key)

	_, err = bundleImageKeyForRegistry("docker.io/bitnami/nginx:1.27.0", "registry.local")
	require.ErrorIs(t, err, ErrUploadValidation)
}

func TestRenderOfflineChartImagesInjectsConfiguredRegistry(t *testing.T) {
	restoreOfflineImageConfig(t, true, "registry.local", true)
	chartData := testChartArchive(t, "demo", "0.1.0", "nginx:1.27.0")
	ch, err := LoadChartArchiveBytes(chartData)
	require.NoError(t, err)

	images, err := RenderOfflineChartImages(ch, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"registry.local/nginx:1.27.0"}, images)
}

func TestImportOfflineBundlePushesImagesBeforeChart(t *testing.T) {
	restoreOfflineImageConfig(t, true, "registry.local", true)
	chartData := testChartArchive(t, "demo", "0.1.0", "nginx:1.27.0")
	imageData := []byte("image archive")
	bundlePath := testOfflineBundle(t, OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "demo",
			Version:      "0.1.0",
			ChartArchive: "charts/demo-0.1.0.tgz",
			ChartDigest:  digestBytes(chartData),
			Images: []OfflineBundleImage{{
				Image:         "nginx:1.27.0",
				Archive:       "images/nginx.tar.gz",
				ArchiveDigest: digestBytes(imageData),
			}},
		}},
	}, map[string][]byte{
		"charts/demo-0.1.0.tgz": chartData,
		"images/nginx.tar.gz":   imageData,
	})

	events := []string{}
	result, err := ImportOfflineBundle(context.Background(), bundlePath, BundleImportOptions{
		PushImage: func(_ context.Context, req ExactContainerImageUploadRequest) (ContainerImageUploadResult, error) {
			events = append(events, "image:"+req.ImageRef)
			return ContainerImageUploadResult{ImageRef: req.ImageRef, Size: req.Size}, nil
		},
		PushChart: func(data []byte) (OCIChartUploadResult, error) {
			events = append(events, "chart")
			return OCIChartUploadResult{ChartName: "demo", Version: "0.1.0", ChartURL: "oci://registry.local/kite-helm/demo:0.1.0", Size: int64(len(data))}, nil
		},
	})

	require.NoError(t, err)
	require.Len(t, result.Apps, 1)
	require.Empty(t, result.Apps[0].Error)
	require.Equal(t, []string{"image:registry.local/nginx:1.27.0", "chart"}, events)
}

func TestImportOfflineBundleDoesNotPushChartWhenRenderedImageMissing(t *testing.T) {
	restoreOfflineImageConfig(t, true, "registry.local", true)
	chartData := testChartArchive(t, "demo", "0.1.0", "nginx:1.27.0")
	bundlePath := testOfflineBundle(t, OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "demo",
			Version:      "0.1.0",
			ChartArchive: "charts/demo-0.1.0.tgz",
			ChartDigest:  digestBytes(chartData),
			Images: []OfflineBundleImage{{
				Image:   "other/image:1.0.0",
				Archive: "images/other.tar.gz",
			}},
		}},
	}, map[string][]byte{
		"charts/demo-0.1.0.tgz": chartData,
		"images/other.tar.gz":   []byte("other"),
	})

	chartPushed := false
	result, err := ImportOfflineBundle(context.Background(), bundlePath, BundleImportOptions{
		PushImage: func(_ context.Context, req ExactContainerImageUploadRequest) (ContainerImageUploadResult, error) {
			return ContainerImageUploadResult{ImageRef: req.ImageRef, Size: req.Size}, nil
		},
		PushChart: func(data []byte) (OCIChartUploadResult, error) {
			chartPushed = true
			return OCIChartUploadResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Len(t, result.Apps, 1)
	require.Contains(t, result.Apps[0].Error, "bundle is missing rendered image nginx:1.27.0")
	require.False(t, chartPushed)
}

func TestImportOfflineBundleRejectsManifestChartMetadataMismatch(t *testing.T) {
	restoreOfflineImageConfig(t, true, "registry.local", true)
	chartData := testChartArchive(t, "demo", "0.1.0", "nginx:1.27.0")
	bundlePath := testOfflineBundle(t, OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "other",
			Version:      "0.1.0",
			ChartArchive: "charts/demo-0.1.0.tgz",
			ChartDigest:  digestBytes(chartData),
			Images: []OfflineBundleImage{{
				Image:   "nginx:1.27.0",
				Archive: "images/nginx.tar.gz",
			}},
		}},
	}, map[string][]byte{
		"charts/demo-0.1.0.tgz": chartData,
		"images/nginx.tar.gz":   []byte("image archive"),
	})

	imagePushed := false
	chartPushed := false
	result, err := ImportOfflineBundle(context.Background(), bundlePath, BundleImportOptions{
		PushImage: func(_ context.Context, req ExactContainerImageUploadRequest) (ContainerImageUploadResult, error) {
			imagePushed = true
			return ContainerImageUploadResult{ImageRef: req.ImageRef, Size: req.Size}, nil
		},
		PushChart: func(data []byte) (OCIChartUploadResult, error) {
			chartPushed = true
			return OCIChartUploadResult{}, nil
		},
	})

	require.NoError(t, err)
	require.Len(t, result.Apps, 1)
	require.Contains(t, result.Apps[0].Error, "does not match bundle manifest")
	require.False(t, imagePushed)
	require.False(t, chartPushed)
}

func TestImportOfflineBundleUsesBundledValuesForRenderedImages(t *testing.T) {
	restoreOfflineImageConfig(t, true, "registry.local", true)
	chartData := testChartArchive(t, "demo", "0.1.0", "{{ .Values.image.repository }}:{{ .Values.image.tag }}")
	imageData := []byte("image archive")
	bundlePath := testOfflineBundle(t, OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "demo",
			Version:      "0.1.0",
			ChartArchive: "charts/demo-0.1.0.tgz",
			ChartDigest:  digestBytes(chartData),
			Values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "redis",
					"tag":        "7.2.0",
				},
			},
			Images: []OfflineBundleImage{{
				Image:         "redis:7.2.0",
				Archive:       "images/redis.tar.gz",
				ArchiveDigest: digestBytes(imageData),
			}},
		}},
	}, map[string][]byte{
		"charts/demo-0.1.0.tgz": chartData,
		"images/redis.tar.gz":   imageData,
	})

	pushedRefs := []string{}
	result, err := ImportOfflineBundle(context.Background(), bundlePath, BundleImportOptions{
		PushImage: func(_ context.Context, req ExactContainerImageUploadRequest) (ContainerImageUploadResult, error) {
			pushedRefs = append(pushedRefs, req.ImageRef)
			return ContainerImageUploadResult{ImageRef: req.ImageRef, Size: req.Size}, nil
		},
		PushChart: func(data []byte) (OCIChartUploadResult, error) {
			return OCIChartUploadResult{ChartName: "demo", Version: "0.1.0", ChartURL: "oci://registry.local/kite-helm/demo:0.1.0", Size: int64(len(data))}, nil
		},
	})

	require.NoError(t, err)
	require.Len(t, result.Apps, 1)
	require.Empty(t, result.Apps[0].Error)
	require.Equal(t, []string{"registry.local/redis:7.2.0"}, pushedRefs)
}

func TestValidateOfflineBundleManifestRejectsUnsafePaths(t *testing.T) {
	err := validateOfflineBundleManifest(OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "demo",
			Version:      "0.1.0",
			ChartArchive: "../demo.tgz",
			Images: []OfflineBundleImage{{
				Image:   "nginx:1.27.0",
				Archive: "images/nginx.tar.gz",
			}},
		}},
	})
	require.ErrorIs(t, err, ErrUploadValidation)
}

func TestValidateOfflineBundleManifestRejectsImageRegistryHost(t *testing.T) {
	err := validateOfflineBundleManifest(OfflineBundleManifest{
		APIVersion: BundleFormat,
		Apps: []OfflineBundleApplication{{
			Name:         "demo",
			Version:      "0.1.0",
			ChartArchive: "charts/demo-0.1.0.tgz",
			Images: []OfflineBundleImage{{
				Image:   "registry.local/nginx:1.27.0",
				Archive: "images/nginx.tar.gz",
			}},
		}},
	})
	require.ErrorIs(t, err, ErrUploadValidation)
	require.Contains(t, err.Error(), "must not include a registry host")
}

func testChartArchive(t *testing.T, name, version, image string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	files := map[string]string{
		path.Join(name, "Chart.yaml"): "apiVersion: v2\nname: " + name + "\nversion: " + version + "\n",
		path.Join(name, "templates", "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}
    spec:
      containers:
        - name: app
          image: {{ with .Values.global.imageRegistry }}{{ . }}/{{ end }}` + image + `
`,
	}
	for fileName, content := range files {
		data := []byte(content)
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: fileName,
			Mode: 0o600,
			Size: int64(len(data)),
		}))
		_, err := tarWriter.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buf.Bytes()
}

func testOfflineBundle(t *testing.T, manifest OfflineBundleManifest, files map[string][]byte) string {
	t.Helper()
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "bundle.kiteapp.tar.gz")
	file, err := os.Create(bundlePath)
	require.NoError(t, err)
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)
	files[BundleManifestFile] = manifestData
	for fileName, data := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: fileName,
			Mode: 0o600,
			Size: int64(len(data)),
		}))
		_, err := tarWriter.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, file.Close())
	return bundlePath
}

func restoreOfflineImageConfig(t *testing.T, enabled bool, registry string, enforce bool) {
	t.Helper()
	originalEnabled := common.HelmOfflineImagesEnabled
	originalRegistry := common.HelmOfflineImagesRegistry
	originalEnforce := common.HelmOfflineImagesEnforce
	common.HelmOfflineImagesEnabled = enabled
	common.HelmOfflineImagesRegistry = registry
	common.HelmOfflineImagesEnforce = enforce
	t.Cleanup(func() {
		common.HelmOfflineImagesEnabled = originalEnabled
		common.HelmOfflineImagesRegistry = originalRegistry
		common.HelmOfflineImagesEnforce = originalEnforce
	})
}
