package helmutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	kitecommon "github.com/zxh326/kite/pkg/common"
	helmcommon "helm.sh/helm/v4/pkg/chart/common"
	commonutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/engine"
)

const (
	BundleManifestFile = "kite-bundle.json"
	BundleFormat       = "kite.offline.application.bundle/v1"
	defaultBundleNS    = "default"
	defaultBundleRel   = "offline-preview"
)

type OfflineBundleManifest struct {
	APIVersion string                     `json:"apiVersion"`
	CreatedAt  string                     `json:"createdAt,omitempty"`
	Apps       []OfflineBundleApplication `json:"apps"`
}

type OfflineBundleApplication struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	ChartArchive string                 `json:"chartArchive"`
	ChartDigest  string                 `json:"chartDigest,omitempty"`
	Values       map[string]interface{} `json:"values,omitempty"`
	Images       []OfflineBundleImage   `json:"images"`
}

type OfflineBundleImage struct {
	Image         string `json:"image"`
	Archive       string `json:"archive"`
	ArchiveDigest string `json:"archiveDigest,omitempty"`
	SourceImage   string `json:"sourceImage,omitempty"`
}

type BundleImportResult struct {
	Apps []BundleImportAppResult `json:"apps"`
}

type BundleImportAppResult struct {
	Name     string                       `json:"name"`
	Version  string                       `json:"version"`
	ChartURL string                       `json:"chartUrl,omitempty"`
	Images   []ContainerImageUploadResult `json:"images,omitempty"`
	Chart    OCIChartUploadResult         `json:"chart,omitempty"`
	Skipped  bool                         `json:"skipped,omitempty"`
	Error    string                       `json:"error,omitempty"`
}

type BundleImportOptions struct {
	MaxBytes  int64
	PushImage func(context.Context, ExactContainerImageUploadRequest) (ContainerImageUploadResult, error)
	PushChart func([]byte) (OCIChartUploadResult, error)
}

type BundleExportOptions struct {
	Apps      []BundleExportApplication
	PullImage func(context.Context, string, string) (string, int64, error)
}

type BundleExportApplication struct {
	RepositoryName string                 `json:"repositoryName"`
	ChartName      string                 `json:"chartName"`
	Version        string                 `json:"version"`
	Values         map[string]interface{} `json:"values,omitempty"`
}

func ImportOfflineBundle(ctx context.Context, bundlePath string, opts BundleImportOptions) (BundleImportResult, error) {
	if opts.PushImage == nil {
		opts.PushImage = PushContainerImageArchiveToRef
	}
	if opts.PushChart == nil {
		opts.PushChart = PushOCIChartArchive
	}
	workDir, manifest, err := extractOfflineBundle(bundlePath, opts.MaxBytes)
	if err != nil {
		return BundleImportResult{}, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	if err := validateOfflineBundleManifest(manifest); err != nil {
		return BundleImportResult{}, err
	}

	result := BundleImportResult{Apps: make([]BundleImportAppResult, 0, len(manifest.Apps))}
	for _, app := range manifest.Apps {
		appResult := BundleImportAppResult{Name: app.Name, Version: app.Version}
		chartPath := filepath.Join(workDir, filepath.FromSlash(app.ChartArchive))
		chartData, err := os.ReadFile(chartPath)
		if err != nil {
			appResult.Error = fmt.Sprintf("failed to read chart archive: %v", err)
			result.Apps = append(result.Apps, appResult)
			continue
		}
		if app.ChartDigest != "" {
			if got := digestBytes(chartData); got != app.ChartDigest {
				appResult.Error = fmt.Sprintf("chart digest mismatch: got %s", got)
				result.Apps = append(result.Apps, appResult)
				continue
			}
		}
		loadedChart, err := LoadChartArchiveBytes(chartData)
		if err != nil {
			appResult.Error = err.Error()
			result.Apps = append(result.Apps, appResult)
			continue
		}
		if loadedChart.Metadata.Name != app.Name || loadedChart.Metadata.Version != app.Version {
			appResult.Error = fmt.Sprintf("chart archive metadata %s:%s does not match bundle manifest %s:%s", loadedChart.Metadata.Name, loadedChart.Metadata.Version, app.Name, app.Version)
			result.Apps = append(result.Apps, appResult)
			continue
		}
		targetImages, err := RenderOfflineChartImages(loadedChart, app.Values)
		if err != nil {
			appResult.Error = err.Error()
			result.Apps = append(result.Apps, appResult)
			continue
		}
		imageByRef := map[string]OfflineBundleImage{}
		for _, image := range app.Images {
			imageByRef[image.Image] = image
		}
		for _, imageRef := range targetImages {
			imageKey, err := bundleImageKeyForRegistry(imageRef, kitecommon.HelmOfflineImagesRegistry)
			if err != nil {
				appResult.Error = err.Error()
				break
			}
			image, ok := imageByRef[imageKey]
			if !ok {
				appResult.Error = fmt.Sprintf("bundle is missing rendered image %s", imageKey)
				break
			}
			archivePath := filepath.Join(workDir, filepath.FromSlash(image.Archive))
			if image.ArchiveDigest != "" {
				got, err := digestFile(archivePath)
				if err != nil {
					appResult.Error = fmt.Sprintf("failed to digest image archive %s: %v", imageRef, err)
					break
				}
				if got != image.ArchiveDigest {
					appResult.Error = fmt.Sprintf("image archive digest mismatch for %s: got %s", imageRef, got)
					break
				}
			}
			stat, err := os.Stat(archivePath)
			if err != nil {
				appResult.Error = fmt.Sprintf("failed to stat image archive %s: %v", imageRef, err)
				break
			}
			pushed, err := opts.PushImage(ctx, ExactContainerImageUploadRequest{
				ArchivePath: archivePath,
				ImageRef:    imageRef,
				Size:        stat.Size(),
				MaxBytes:    opts.MaxBytes,
			})
			if err != nil {
				appResult.Error = fmt.Sprintf("failed to push image %s: %v", imageRef, err)
				break
			}
			appResult.Images = append(appResult.Images, pushed)
		}
		if appResult.Error != "" {
			result.Apps = append(result.Apps, appResult)
			continue
		}
		chartResult, err := opts.PushChart(chartData)
		if err != nil {
			appResult.Error = fmt.Sprintf("failed to push chart: %v", err)
			result.Apps = append(result.Apps, appResult)
			continue
		}
		appResult.Chart = chartResult
		appResult.ChartURL = chartResult.ChartURL
		result.Apps = append(result.Apps, appResult)
	}
	return result, nil
}

func ExportOfflineBundle(ctx context.Context, outputPath string, opts BundleExportOptions) (OfflineBundleManifest, error) {
	if opts.PullImage == nil {
		opts.PullImage = PullContainerImageArchiveToFile
	}
	if len(opts.Apps) == 0 {
		return OfflineBundleManifest{}, fmt.Errorf("%w: at least one app is required", ErrUploadValidation)
	}
	workDir, err := os.MkdirTemp("", "kite-offline-bundle-*")
	if err != nil {
		return OfflineBundleManifest{}, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	chartsDir := filepath.Join(workDir, "charts")
	imagesDir := filepath.Join(workDir, "images")
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		return OfflineBundleManifest{}, err
	}
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return OfflineBundleManifest{}, err
	}

	manifest := OfflineBundleManifest{
		APIVersion: BundleFormat,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	for _, requested := range opts.Apps {
		ref, err := FindOCIChartVersion(requested.RepositoryName, requested.ChartName, requested.Version)
		if err != nil {
			return OfflineBundleManifest{}, err
		}
		loadedChart, err := LoadOCIArchive(ref)
		if err != nil {
			return OfflineBundleManifest{}, err
		}
		chartPath, err := chartutil.Save(loadedChart, chartsDir)
		if err != nil {
			return OfflineBundleManifest{}, err
		}
		chartData, err := os.ReadFile(chartPath)
		if err != nil {
			return OfflineBundleManifest{}, err
		}
		images, err := RenderOfflineChartImages(loadedChart, requested.Values)
		if err != nil {
			return OfflineBundleManifest{}, err
		}
		app := OfflineBundleApplication{
			Name:         loadedChart.Metadata.Name,
			Version:      loadedChart.Metadata.Version,
			ChartArchive: path.Join("charts", filepath.Base(chartPath)),
			ChartDigest:  digestBytes(chartData),
			Values:       cloneValuesMap(requested.Values),
		}
		for _, imageRef := range images {
			imageKey, err := bundleImageKeyForRegistry(imageRef, kitecommon.HelmOfflineImagesRegistry)
			if err != nil {
				return OfflineBundleManifest{}, err
			}
			imageFile := imageArchiveFileName(imageKey)
			imagePath := filepath.Join(imagesDir, imageFile)
			if _, _, err := opts.PullImage(ctx, imageRef, imagePath); err != nil {
				return OfflineBundleManifest{}, fmt.Errorf("failed to pull image %s: %w", imageRef, err)
			}
			digest, err := digestFile(imagePath)
			if err != nil {
				return OfflineBundleManifest{}, err
			}
			app.Images = append(app.Images, OfflineBundleImage{
				Image:         imageKey,
				Archive:       path.Join("images", imageFile),
				ArchiveDigest: digest,
				SourceImage:   imageRef,
			})
		}
		manifest.Apps = append(manifest.Apps, app)
	}
	if err := writeOfflineBundle(workDir, outputPath, manifest); err != nil {
		return OfflineBundleManifest{}, err
	}
	return manifest, nil
}

func LoadChartArchiveBytes(data []byte) (*chart.Chart, error) {
	loadedChart, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load Helm chart archive: %v", ErrUploadValidation, err)
	}
	if err := validateUploadChartMetadata(loadedChart.Metadata); err != nil {
		return nil, err
	}
	return loadedChart, nil
}

func RenderOfflineChartImages(ch *chart.Chart, values map[string]interface{}) ([]string, error) {
	policy := OfflineImagePolicy{
		Enabled:  kitecommon.HelmOfflineImagesEnabled || strings.TrimSpace(kitecommon.HelmOfflineImagesRegistry) != "",
		Registry: strings.TrimSpace(kitecommon.HelmOfflineImagesRegistry),
		Enforce:  true,
	}
	if policy.Registry == "" {
		return nil, fmt.Errorf("%w: KITE_HELM_OFFLINE_IMAGE_REGISTRY is required", ErrUploadNotConfigured)
	}
	prepared, _ := ApplyOfflineImagePolicy(values, policy)
	rendered, err := renderChartManifest(ch, prepared)
	if err != nil {
		return nil, err
	}
	images := ExtractManifestImages(rendered)
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: chart renders no workload images", ErrUploadValidation)
	}
	external := []string{}
	for _, image := range images {
		if !imageUsesRegistry(image, policy.Registry) {
			external = append(external, image)
		}
	}
	if len(external) > 0 {
		return nil, fmt.Errorf("%w: rendered images do not use %s: %s", ErrUploadValidation, policy.Registry, strings.Join(external, ", "))
	}
	return images, nil
}

func PullContainerImageArchiveToFile(ctx context.Context, imageRef, outputPath string) (string, int64, error) {
	config, err := loadExactImageUploadConfig()
	if err != nil {
		return "", 0, err
	}
	if !config.Configured {
		return "", 0, fmt.Errorf("%w: %s or %s is required", ErrUploadNotConfigured, imageUploadRegistryEnv, "KITE_HELM_OFFLINE_IMAGE_REGISTRY")
	}
	ref, err := parseExactImageUploadReference(config, imageRef)
	if err != nil {
		return "", 0, err
	}
	remoteOptions, err := imageRemoteOptions(config.Options, ctx)
	if err != nil {
		return "", 0, err
	}
	img, err := remote.Image(ref, remoteOptions...)
	if err != nil {
		return "", 0, err
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return "", 0, err
	}
	gzipWriter := gzip.NewWriter(file)
	writeErr := tarball.Write(ref, img, gzipWriter)
	closeGzipErr := gzipWriter.Close()
	closeFileErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(outputPath)
		return "", 0, writeErr
	}
	if closeGzipErr != nil {
		_ = os.Remove(outputPath)
		return "", 0, closeGzipErr
	}
	if closeFileErr != nil {
		_ = os.Remove(outputPath)
		return "", 0, closeFileErr
	}
	stat, err := os.Stat(outputPath)
	if err != nil {
		return "", 0, err
	}
	digest, err := img.Digest()
	if err != nil {
		return "", 0, err
	}
	return digest.String(), stat.Size(), nil
}

func renderChartManifest(ch *chart.Chart, values map[string]interface{}) (string, error) {
	if values == nil {
		values = map[string]interface{}{}
	}
	if err := chartutil.ProcessDependencies(ch, helmcommon.Values(values)); err != nil {
		return "", err
	}
	renderValues, err := commonutil.ToRenderValuesWithSchemaValidation(ch, values, helmcommon.ReleaseOptions{
		Name:      defaultBundleRel,
		Namespace: defaultBundleNS,
		Revision:  1,
		IsInstall: true,
	}, helmcommon.DefaultCapabilities.Copy(), false)
	if err != nil {
		return "", err
	}
	rendered, err := engine.Render(ch, renderValues)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(rendered))
	for key := range rendered {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		content := strings.TrimSpace(rendered[key])
		if content == "" {
			continue
		}
		builder.WriteString("\n---\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func extractOfflineBundle(bundlePath string, maxBytes int64) (string, OfflineBundleManifest, error) {
	workDir, err := os.MkdirTemp("", "kite-offline-bundle-import-*")
	if err != nil {
		return "", OfflineBundleManifest{}, err
	}
	if err := extractTarToDir(bundlePath, workDir, maxBytes); err != nil {
		_ = os.RemoveAll(workDir)
		return "", OfflineBundleManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(workDir, BundleManifestFile))
	if err != nil {
		_ = os.RemoveAll(workDir)
		return "", OfflineBundleManifest{}, fmt.Errorf("%w: bundle manifest is required", ErrUploadValidation)
	}
	var manifest OfflineBundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		_ = os.RemoveAll(workDir)
		return "", OfflineBundleManifest{}, fmt.Errorf("%w: invalid bundle manifest: %v", ErrUploadValidation, err)
	}
	return workDir, manifest, nil
}

func writeOfflineBundle(workDir, outputPath string, manifest OfflineBundleManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(workDir, BundleManifestFile), append(data, '\n'), 0o600); err != nil {
		return err
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.WalkDir(workDir, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workDir, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		in, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeTarErr := tarWriter.Close()
	closeGzipErr := gzipWriter.Close()
	closeFileErr := file.Close()
	if walkErr != nil {
		_ = os.Remove(outputPath)
		return walkErr
	}
	if closeTarErr != nil {
		_ = os.Remove(outputPath)
		return closeTarErr
	}
	if closeGzipErr != nil {
		_ = os.Remove(outputPath)
		return closeGzipErr
	}
	if closeFileErr != nil {
		_ = os.Remove(outputPath)
		return closeFileErr
	}
	return nil
}

func bundleImageKeyForRegistry(imageRef, registry string) (string, error) {
	imageRef = strings.TrimSpace(imageRef)
	registry = cleanRegistryHost(registry)
	if imageRef == "" {
		return "", fmt.Errorf("%w: image reference is required", ErrUploadValidation)
	}
	if registry == "" {
		return "", fmt.Errorf("%w: KITE_HELM_OFFLINE_IMAGE_REGISTRY is required", ErrUploadNotConfigured)
	}
	prefix := registry + "/"
	if !strings.HasPrefix(imageRef, prefix) {
		return "", fmt.Errorf("%w: image reference must use configured registry %s", ErrUploadValidation, registry)
	}
	key := strings.TrimPrefix(imageRef, prefix)
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "://") || strings.Contains(key, "..") {
		return "", fmt.Errorf("%w: invalid bundle image reference %q", ErrUploadValidation, imageRef)
	}
	if err := validateBundleImageReference(key); err != nil {
		return "", err
	}
	return key, nil
}

func validateOfflineBundleManifest(manifest OfflineBundleManifest) error {
	if manifest.APIVersion != BundleFormat {
		return fmt.Errorf("%w: unsupported bundle apiVersion %q", ErrUploadValidation, manifest.APIVersion)
	}
	if len(manifest.Apps) == 0 {
		return fmt.Errorf("%w: bundle must include at least one app", ErrUploadValidation)
	}
	seen := map[string]struct{}{}
	for _, app := range manifest.Apps {
		if strings.TrimSpace(app.Name) == "" || strings.TrimSpace(app.Version) == "" {
			return fmt.Errorf("%w: app name and version are required", ErrUploadValidation)
		}
		key := app.Name + ":" + app.Version
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate app %s", ErrUploadValidation, key)
		}
		seen[key] = struct{}{}
		if err := validateBundlePath(app.ChartArchive, "charts/"); err != nil {
			return err
		}
		if len(app.Images) == 0 {
			return fmt.Errorf("%w: app %s must include at least one image", ErrUploadValidation, key)
		}
		seenImages := map[string]struct{}{}
		for _, image := range app.Images {
			if strings.TrimSpace(image.Image) == "" {
				return fmt.Errorf("%w: image reference is required for app %s", ErrUploadValidation, key)
			}
			if err := validateBundleImageReference(image.Image); err != nil {
				return err
			}
			if _, ok := seenImages[image.Image]; ok {
				return fmt.Errorf("%w: duplicate image %s for app %s", ErrUploadValidation, image.Image, key)
			}
			seenImages[image.Image] = struct{}{}
			if err := validateBundlePath(image.Archive, "images/"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBundleImageReference(image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("%w: bundle image reference is required", ErrUploadValidation)
	}
	if strings.HasPrefix(image, "/") || strings.Contains(image, "://") || strings.Contains(image, "..") {
		return fmt.Errorf("%w: bundle image reference must be relative to the offline image registry", ErrUploadValidation)
	}
	firstSegment := strings.Split(image, "/")[0]
	if strings.EqualFold(firstSegment, "localhost") || strings.HasPrefix(strings.ToLower(firstSegment), "localhost:") {
		return fmt.Errorf("%w: bundle image reference must not include a registry host", ErrUploadValidation)
	}
	host := firstSegment
	if before, _, ok := strings.Cut(firstSegment, ":"); ok {
		host = before
		if strings.Contains(image, "/") {
			return fmt.Errorf("%w: bundle image reference must not include a registry host", ErrUploadValidation)
		}
	}
	if strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return fmt.Errorf("%w: bundle image reference must not include a registry host", ErrUploadValidation)
	}
	return nil
}

func validateBundlePath(value, prefix string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%w: bundle path is required", ErrUploadValidation)
	}
	clean := path.Clean(value)
	if clean != value || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == "." || clean == ".." {
		return fmt.Errorf("%w: unsafe bundle path %q", ErrUploadValidation, value)
	}
	if !strings.HasPrefix(clean, prefix) {
		return fmt.Errorf("%w: bundle path %q must be under %s", ErrUploadValidation, value, prefix)
	}
	return nil
}

func imageArchiveFileName(imageRef string) string {
	sum := sha256.Sum256([]byte(imageRef))
	return hex.EncodeToString(sum[:]) + ".tar.gz"
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
