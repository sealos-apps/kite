package helmutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/zxh326/kite/pkg/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/registry"
)

const (
	ociChartUploadMaxBytesEnv = "KITE_HELM_OCI_UPLOAD_MAX_BYTES"

	imageUploadRegistryEnv           = "KITE_IMAGE_UPLOAD_REGISTRY"
	imageUploadRepositoryPrefixEnv   = "KITE_IMAGE_UPLOAD_REPOSITORY_PREFIX"
	imageUploadMaxBytesEnv           = "KITE_IMAGE_UPLOAD_MAX_BYTES"
	imageUploadPlainHTTPEnv          = "KITE_IMAGE_UPLOAD_REGISTRY_PLAIN_HTTP"
	imageUploadInsecureTLSEnv        = "KITE_IMAGE_UPLOAD_REGISTRY_INSECURE_SKIP_TLS_VERIFY"
	imageUploadCAFileEnv             = "KITE_IMAGE_UPLOAD_REGISTRY_CA_FILE"
	imageUploadUsernameEnv           = "KITE_IMAGE_UPLOAD_REGISTRY_USERNAME"
	imageUploadPasswordEnv           = "KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD"
	defaultOCIChartUploadMaxBytes    = 512 * 1024 * 1024
	defaultImageUploadMaxBytes       = 4 * 1024 * 1024 * 1024
	defaultImageUploadRepositoryPath = "kite-images"
)

var (
	ErrUploadNotConfigured = errors.New("repository upload is not configured")
	ErrUploadValidation    = errors.New("invalid repository upload")
)

type RepositoryUploadConfig struct {
	Chart OCIChartUploadConfig       `json:"chart"`
	Image ContainerImageUploadConfig `json:"image"`
}

type OCIChartUploadConfig struct {
	Configured     bool   `json:"configured"`
	RegistryBase   string `json:"registryBase,omitempty"`
	RepositoryName string `json:"repositoryName,omitempty"`
	MaxBytes       int64  `json:"maxBytes"`
}

type ContainerImageUploadConfig struct {
	Configured       bool   `json:"configured"`
	Registry         string `json:"registry,omitempty"`
	RepositoryPrefix string `json:"repositoryPrefix,omitempty"`
	MaxBytes         int64  `json:"maxBytes"`
}

type OCIChartUploadResult struct {
	RepositoryName string `json:"repositoryName"`
	ChartName      string `json:"chartName"`
	Version        string `json:"version"`
	ChartURL       string `json:"chartUrl"`
	PushedRef      string `json:"pushedRef"`
	Digest         string `json:"digest,omitempty"`
	Size           int64  `json:"size"`
}

type ContainerImageUploadResult struct {
	ImageRef string `json:"imageRef"`
	Digest   string `json:"digest,omitempty"`
	Size     int64  `json:"size"`
}

type ContainerImageUploadRequest struct {
	ArchivePath string
	Repository  string
	Tag         string
	Size        int64
	MaxBytes    int64
}

type imageUploadConfig struct {
	ContainerImageUploadConfig
	Options OCIRegistryOptions
}

func LoadRepositoryUploadConfig() (RepositoryUploadConfig, error) {
	chartConfig, err := LoadOCIChartUploadConfig()
	if err != nil {
		return RepositoryUploadConfig{}, err
	}
	imageConfig, err := LoadContainerImageUploadConfig()
	if err != nil {
		return RepositoryUploadConfig{}, err
	}
	return RepositoryUploadConfig{Chart: chartConfig, Image: imageConfig}, nil
}

func LoadOCIChartUploadConfig() (OCIChartUploadConfig, error) {
	maxBytes, err := OCIChartUploadMaxBytes()
	if err != nil {
		return OCIChartUploadConfig{}, err
	}
	config, err := loadOCIRegistryDiscoveryConfig()
	if err != nil {
		return OCIChartUploadConfig{}, err
	}
	return OCIChartUploadConfig{
		Configured:     config.Enabled,
		RegistryBase:   config.BaseURL,
		RepositoryName: config.RepositoryName,
		MaxBytes:       maxBytes,
	}, nil
}

func LoadContainerImageUploadConfig() (ContainerImageUploadConfig, error) {
	config, err := loadImageUploadConfig()
	if err != nil {
		return ContainerImageUploadConfig{}, err
	}
	return config.ContainerImageUploadConfig, nil
}

func OCIChartUploadMaxBytes() (int64, error) {
	return parsePositiveInt64Env(ociChartUploadMaxBytesEnv, defaultOCIChartUploadMaxBytes)
}

func ContainerImageUploadMaxBytes() (int64, error) {
	return parsePositiveInt64Env(imageUploadMaxBytesEnv, defaultImageUploadMaxBytes)
}

func PushOCIChartArchive(data []byte) (OCIChartUploadResult, error) {
	config, err := loadOCIRegistryDiscoveryConfig()
	if err != nil {
		return OCIChartUploadResult{}, err
	}
	if !config.Enabled {
		return OCIChartUploadResult{}, fmt.Errorf("%w: %s is required", ErrUploadNotConfigured, ociRegistryBaseEnv)
	}
	loadedChart, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return OCIChartUploadResult{}, fmt.Errorf("%w: failed to load Helm chart archive: %v", ErrUploadValidation, err)
	}
	meta := loadedChart.Metadata
	if err := validateUploadChartMetadata(meta); err != nil {
		return OCIChartUploadResult{}, err
	}

	client, err := newOCIRegistryClient(config.RegistryOptions)
	if err != nil {
		return OCIChartUploadResult{}, err
	}
	chartURL := strings.TrimRight(config.BaseURL, "/") + "/" + meta.Name + ":" + meta.Version
	pushRef := strings.TrimPrefix(chartURL, registry.OCIScheme+"://")
	result, err := client.Push(data, pushRef)
	if err != nil {
		return OCIChartUploadResult{}, err
	}
	ClearOCIChartDiscoveryCache()

	pushedRef := result.Ref
	if pushedRef != "" {
		pushedRef = registry.OCIScheme + "://" + pushedRef
	}
	digest := ""
	if result.Manifest != nil {
		digest = result.Manifest.Digest
	}
	return OCIChartUploadResult{
		RepositoryName: config.RepositoryName,
		ChartName:      meta.Name,
		Version:        meta.Version,
		ChartURL:       chartURL,
		PushedRef:      pushedRef,
		Digest:         digest,
		Size:           int64(len(data)),
	}, nil
}

func PushContainerImageArchive(ctx context.Context, req ContainerImageUploadRequest) (ContainerImageUploadResult, error) {
	config, err := loadImageUploadConfig()
	if err != nil {
		return ContainerImageUploadResult{}, err
	}
	if !config.Configured {
		return ContainerImageUploadResult{}, fmt.Errorf("%w: %s or %s is required", ErrUploadNotConfigured, imageUploadRegistryEnv, "KITE_HELM_OFFLINE_IMAGE_REGISTRY")
	}
	ref, err := buildImageUploadReference(config, req.Repository, req.Tag)
	if err != nil {
		return ContainerImageUploadResult{}, err
	}
	remoteOptions, err := imageRemoteOptions(config.Options, ctx)
	if err != nil {
		return ContainerImageUploadResult{}, err
	}

	img, err := tarball.Image(imageArchiveOpener(req.ArchivePath), nil)
	if err == nil {
		digest, err := pushContainerImage(ctx, ref, img, remoteOptions)
		if err != nil {
			return ContainerImageUploadResult{}, err
		}
		return ContainerImageUploadResult{ImageRef: ref.Name(), Digest: digest, Size: req.Size}, nil
	}
	dockerArchiveErr := err

	maxExtractBytes := req.MaxBytes
	if maxExtractBytes <= 0 {
		maxExtractBytes = req.Size
	}
	index, err := imageIndexFromOCIArchive(req.ArchivePath, maxExtractBytes)
	if err != nil {
		return ContainerImageUploadResult{}, fmt.Errorf("%w: failed to load image archive as docker save tarball (%v) or OCI layout archive (%v)", ErrUploadValidation, dockerArchiveErr, err)
	}
	digest, err := pushImageIndex(ctx, ref, index, remoteOptions)
	if err != nil {
		return ContainerImageUploadResult{}, err
	}
	return ContainerImageUploadResult{ImageRef: ref.Name(), Digest: digest, Size: req.Size}, nil
}

func validateUploadChartMetadata(meta *chart.Metadata) error {
	if meta == nil {
		return fmt.Errorf("%w: Chart.yaml metadata is missing", ErrUploadValidation)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return fmt.Errorf("%w: chart name is required", ErrUploadValidation)
	}
	if strings.Contains(meta.Name, "/") {
		return fmt.Errorf("%w: chart name cannot contain /", ErrUploadValidation)
	}
	if strings.TrimSpace(meta.Version) == "" {
		return fmt.Errorf("%w: chart version is required", ErrUploadValidation)
	}
	return nil
}

func loadImageUploadConfig() (imageUploadConfig, error) {
	maxBytes, err := ContainerImageUploadMaxBytes()
	if err != nil {
		return imageUploadConfig{}, err
	}
	options, err := loadImageUploadRegistryOptions()
	if err != nil {
		return imageUploadConfig{}, err
	}
	registryHost := strings.TrimSpace(os.Getenv(imageUploadRegistryEnv))
	if registryHost == "" {
		registryHost = common.HelmOfflineImagesRegistry
	}
	registryHost = cleanRegistryHost(registryHost)
	repositoryPrefix := strings.Trim(strings.TrimSpace(os.Getenv(imageUploadRepositoryPrefixEnv)), "/")
	if repositoryPrefix == "" {
		repositoryPrefix = defaultImageUploadRepositoryPath
	}
	return imageUploadConfig{
		ContainerImageUploadConfig: ContainerImageUploadConfig{
			Configured:       registryHost != "",
			Registry:         registryHost,
			RepositoryPrefix: repositoryPrefix,
			MaxBytes:         maxBytes,
		},
		Options: options,
	}, nil
}

func loadImageUploadRegistryOptions() (OCIRegistryOptions, error) {
	plainHTTP, err := parseOptionalBoolEnv(imageUploadPlainHTTPEnv)
	if err != nil {
		return OCIRegistryOptions{}, err
	}
	insecureSkipTLSVerify, err := parseOptionalBoolEnv(imageUploadInsecureTLSEnv)
	if err != nil {
		return OCIRegistryOptions{}, err
	}
	return OCIRegistryOptions{
		PlainHTTP:             plainHTTP,
		InsecureSkipTLSVerify: insecureSkipTLSVerify,
		CAFile:                strings.TrimSpace(os.Getenv(imageUploadCAFileEnv)),
		Username:              strings.TrimSpace(os.Getenv(imageUploadUsernameEnv)),
		Password:              os.Getenv(imageUploadPasswordEnv),
	}, nil
}

func buildImageUploadReference(config imageUploadConfig, repository, tag string) (name.Reference, error) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	tag = strings.TrimSpace(tag)
	if repository == "" {
		return nil, fmt.Errorf("%w: image repository is required", ErrUploadValidation)
	}
	if tag == "" {
		return nil, fmt.Errorf("%w: image tag is required", ErrUploadValidation)
	}
	if strings.Contains(repository, "://") || strings.Contains(repository, "@") || strings.Contains(repository, ":") {
		return nil, fmt.Errorf("%w: image repository must be a relative repository path without registry, tag, or digest", ErrUploadValidation)
	}
	if strings.Contains(repository, "..") {
		return nil, fmt.Errorf("%w: image repository cannot contain ..", ErrUploadValidation)
	}
	if first := strings.Split(repository, "/")[0]; strings.Contains(first, ".") || first == "localhost" || net.ParseIP(first) != nil {
		return nil, fmt.Errorf("%w: image repository must not include a registry host", ErrUploadValidation)
	}
	fullRepository := strings.Trim(config.RepositoryPrefix, "/")
	if fullRepository != "" {
		fullRepository = path.Join(fullRepository, repository)
	} else {
		fullRepository = repository
	}
	ref := config.Registry + "/" + fullRepository + ":" + tag
	nameOptions := []name.Option{name.StrictValidation}
	if config.Options.PlainHTTP {
		nameOptions = append(nameOptions, name.Insecure)
	}
	parsed, err := name.ParseReference(ref, nameOptions...)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid image reference: %v", ErrUploadValidation, err)
	}
	return parsed, nil
}

func imageRemoteOptions(options OCIRegistryOptions, ctx context.Context) ([]remote.Option, error) {
	remoteOptions := []remote.Option{remote.WithContext(ctx)}
	if options.Username != "" {
		remoteOptions = append(remoteOptions, remote.WithAuth(&authn.Basic{
			Username: options.Username,
			Password: options.Password,
		}))
	}
	if !options.PlainHTTP && (options.InsecureSkipTLSVerify || strings.TrimSpace(options.CAFile) != "") {
		tlsConfig, err := newOCITLSConfig(options)
		if err != nil {
			return nil, err
		}
		remoteOptions = append(remoteOptions, remote.WithTransport(&http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		}))
	}
	return remoteOptions, nil
}

func pushContainerImage(ctx context.Context, ref name.Reference, img v1.Image, options []remote.Option) (string, error) {
	if err := remote.Write(ref, img, options...); err != nil {
		return "", err
	}
	digest, err := img.Digest()
	if err != nil {
		return "", err
	}
	return digest.String(), ctx.Err()
}

func pushImageIndex(ctx context.Context, ref name.Reference, index v1.ImageIndex, options []remote.Option) (string, error) {
	if err := remote.WriteIndex(ref, index, options...); err != nil {
		return "", err
	}
	digest, err := index.Digest()
	if err != nil {
		return "", err
	}
	return digest.String(), ctx.Err()
}

func imageIndexFromOCIArchive(archivePath string, maxExtractBytes int64) (v1.ImageIndex, error) {
	tempDir, err := os.MkdirTemp("", "kite-oci-layout-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	if err := extractTarToDir(archivePath, tempDir, maxExtractBytes); err != nil {
		return nil, err
	}
	return layout.ImageIndexFromPath(tempDir)
}

func extractTarToDir(archivePath, destDir string, maxExtractBytes int64) error {
	file, err := openImageArchive(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	reader := tar.NewReader(file)
	var extractedBytes int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		targetPath, err := safeTarTargetPath(destDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			extractedBytes += header.Size
			if maxExtractBytes > 0 && extractedBytes > maxExtractBytes {
				return fmt.Errorf("extracted OCI layout exceeds %s", formatByteSize(maxExtractBytes))
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}
}

func imageArchiveOpener(archivePath string) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return openImageArchive(archivePath)
	}
}

func openImageArchive(archivePath string) (io.ReadCloser, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	gzipReader, err := gzip.NewReader(file)
	if err == nil {
		return archiveReadCloser{
			Reader: gzipReader,
			close: func() error {
				closeErr := gzipReader.Close()
				fileErr := file.Close()
				if closeErr != nil {
					return closeErr
				}
				return fileErr
			},
		}, nil
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		_ = file.Close()
		return nil, seekErr
	}
	if errors.Is(err, gzip.ErrHeader) {
		return file, nil
	}
	_ = file.Close()
	return nil, err
}

type archiveReadCloser struct {
	io.Reader
	close func() error
}

func (r archiveReadCloser) Close() error {
	return r.close()
}

func safeTarTargetPath(destDir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty tar entry name")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("unsafe tar entry path %q", name)
	}
	targetPath := filepath.Join(destDir, clean)
	if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(filepath.Separator)) && targetPath != filepath.Clean(destDir) {
		return "", fmt.Errorf("unsafe tar entry path %q", name)
	}
	return targetPath, nil
}

func formatByteSize(bytes int64) string {
	const mib = 1024 * 1024
	if bytes%mib == 0 {
		return fmt.Sprintf("%dMiB", bytes/mib)
	}
	return fmt.Sprintf("%d bytes", bytes)
}

func cleanRegistryHost(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return strings.Trim(value, "/")
}

func parsePositiveInt64Env(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := parseByteSize(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s: must be a positive byte size", name)
	}
	return parsed, nil
}

func parseByteSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty byte size")
	}
	multiplier := int64(1)
	lower := strings.ToLower(value)
	for _, suffix := range []struct {
		label string
		value int64
	}{
		{"gib", 1024 * 1024 * 1024},
		{"gb", 1000 * 1000 * 1000},
		{"mib", 1024 * 1024},
		{"mb", 1000 * 1000},
		{"kib", 1024},
		{"kb", 1000},
	} {
		if strings.HasSuffix(lower, suffix.label) {
			multiplier = suffix.value
			value = strings.TrimSpace(value[:len(value)-len(suffix.label)])
			break
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed * multiplier, nil
}

func IsUploadClientError(err error) bool {
	return errors.Is(err, ErrUploadNotConfigured) || errors.Is(err, ErrUploadValidation)
}
