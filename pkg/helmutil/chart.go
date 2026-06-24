package helmutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

const archiveCacheTTL = 10 * time.Minute

var (
	archiveCacheMu sync.Mutex
	archiveCache   = map[string]cachedArchive{}
)

type cachedArchive struct {
	data      []byte
	expiresAt time.Time
}

func LoadRepositoryArchive(repository model.HelmRepository, entry *repo.ChartVersion) (*chart.Chart, error) {
	if len(entry.URLs) == 0 {
		return nil, nil
	}
	chartURL, err := repo.ResolveReferenceURL(repository.URL, entry.URLs[0])
	if err != nil {
		return nil, err
	}
	return LoadArchive(chartURL, &repository)
}

func LoadArchive(chartURL string, repository *model.HelmRepository) (*chart.Chart, error) {
	return loadArchive(chartURL, repository, nil)
}

func LoadOCIArchive(ref OCIChartVersionRef) (*chart.Chart, error) {
	return loadArchive(ref.ChartURL, nil, &ref.Registry)
}

func loadArchive(chartURL string, repository *model.HelmRepository, ociRegistryOptions *OCIRegistryOptions) (*chart.Chart, error) {
	chartURL = strings.TrimSpace(chartURL)
	parsedURL, err := url.Parse(chartURL)
	if err != nil || parsedURL.Scheme == "" {
		return nil, fmt.Errorf("chartUrl must be an absolute URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && parsedURL.Scheme != "oci" {
		return nil, fmt.Errorf("unsupported chartUrl scheme")
	}

	cacheKey := archiveCacheKey(chartURL)
	now := time.Now()
	archiveCacheMu.Lock()
	cached, ok := archiveCache[cacheKey]
	if ok && now.Before(cached.expiresAt) {
		data := append([]byte(nil), cached.data...)
		archiveCacheMu.Unlock()
		return loader.LoadArchive(bytes.NewReader(data))
	}
	archiveCacheMu.Unlock()

	client, err := getter.Getters().ByScheme(parsedURL.Scheme)
	if err != nil {
		return nil, err
	}

	options := []getter.Option{
		getter.WithAcceptHeader("application/gzip,application/octet-stream"),
	}
	useRepositoryCredentials := repository != nil && repository.Username != "" && sameURLHost(repository.URL, chartURL)
	if useRepositoryCredentials {
		options = append(options, getter.WithBasicAuth(repository.Username, string(repository.Password)))
	}

	if parsedURL.Scheme == "oci" {
		optionsFromCatalog, err := registryOptionsForArchive(chartURL, repository, useRepositoryCredentials, ociRegistryOptions)
		if err != nil {
			return nil, err
		}
		registryClient, err := newOCIRegistryClient(optionsFromCatalog)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(path.Base(parsedURL.Path), ":") && !strings.Contains(parsedURL.Path, "@") {
			tags, err := registryClient.Tags(strings.TrimPrefix(chartURL, "oci://"))
			if err != nil {
				return nil, err
			}
			tag, err := registry.GetTagMatchingVersionOrConstraint(tags, "")
			if err != nil {
				return nil, err
			}
			chartURL = chartURL + ":" + tag
		}
		options = append(options, getter.WithRegistryClient(registryClient))
	}

	baseURL := chartURL
	if repository != nil {
		baseURL = repository.URL
	}
	options = append(options, getter.WithURL(baseURL))

	data, err := client.Get(chartURL, options...)
	if err != nil {
		return nil, err
	}
	archiveData := data.Bytes()
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archiveData))
	if err != nil {
		return nil, err
	}

	archiveCacheMu.Lock()
	archiveCache[cacheKey] = cachedArchive{
		data:      append([]byte(nil), archiveData...),
		expiresAt: time.Now().Add(archiveCacheTTL),
	}
	archiveCacheMu.Unlock()

	return loadedChart, nil
}

func registryOptionsForArchive(chartURL string, repository *model.HelmRepository, useRepositoryCredentials bool, explicitOptions *OCIRegistryOptions) (OCIRegistryOptions, error) {
	options := OCIRegistryOptions{}
	if explicitOptions != nil {
		options = *explicitOptions
	} else {
		matched, ok, err := OCIRegistryOptionsForChartURL(chartURL)
		if err != nil {
			return OCIRegistryOptions{}, err
		}
		if ok {
			options = matched
		}
	}
	if useRepositoryCredentials && repository != nil {
		options.Username = repository.Username
		options.Password = string(repository.Password)
	}
	return options, nil
}

func newOCIRegistryClient(options OCIRegistryOptions) (*registry.Client, error) {
	registryOptions := []registry.ClientOption{}
	if options.Username != "" {
		registryOptions = append(registryOptions, registry.ClientOptBasicAuth(options.Username, options.Password))
	}
	if options.PlainHTTP {
		registryOptions = append(registryOptions, registry.ClientOptPlainHTTP())
	}
	if options.InsecureSkipTLSVerify || strings.TrimSpace(options.CAFile) != "" {
		tlsConfig, err := newOCITLSConfig(options)
		if err != nil {
			return nil, err
		}
		registryOptions = append(registryOptions, registry.ClientOptHTTPClient(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				Proxy:           http.ProxyFromEnvironment,
			},
		}))
	}
	return registry.NewClient(registryOptions...)
}

func newOCITLSConfig(options OCIRegistryOptions) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: options.InsecureSkipTLSVerify, //nolint:gosec // User-configured for private offline registries.
	}
	caFile := strings.TrimSpace(options.CAFile)
	if caFile == "" {
		return tlsConfig, nil
	}
	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}
	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	if ok := certPool.AppendCertsFromPEM(caData); !ok {
		return nil, fmt.Errorf("failed to append OCI registry CA certificate")
	}
	tlsConfig.RootCAs = certPool
	return tlsConfig, nil
}

func ValidateChartURLSource(chartURL string, repository *model.HelmRepository, source string) error {
	chartURL = strings.TrimSpace(chartURL)
	parsedURL, err := url.Parse(chartURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("chartUrl must be an absolute URL")
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "oci" {
		return fmt.Errorf("unsupported chartUrl scheme")
	}
	if source == ChartSourceArtifactHub {
		if scheme != "https" || !strings.EqualFold(parsedURL.Hostname(), "artifacthub.io") {
			return fmt.Errorf("artifacthub chartUrl must come from artifacthub.io")
		}
		return nil
	}
	if source == ChartSourceOCI {
		if scheme != "oci" {
			return fmt.Errorf("oci chartUrl must use oci scheme")
		}
		return nil
	}
	if repository == nil {
		return fmt.Errorf("repositoryName is required for repository charts")
	}
	if !sameURLHost(repository.URL, chartURL) {
		return fmt.Errorf("chartUrl host must match repository URL")
	}
	return nil
}

func ResolveURL(baseURL, refURL string) string {
	if refURL == "" {
		return ""
	}
	resolved, err := repo.ResolveReferenceURL(baseURL, refURL)
	if err != nil {
		return refURL
	}
	return resolved
}

func sameURLHost(baseURL, targetURL string) bool {
	base, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	target, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Hostname(), target.Hostname())
}

func archiveCacheKey(chartURL string) string {
	return chartURL
}

func ClearRepositoryArchiveCache(repository model.HelmRepository) {
	cacheKey := repository.URL
	cacheKeyPrefix := strings.TrimRight(cacheKey, "/") + "/"

	archiveCacheMu.Lock()
	for key := range archiveCache {
		if key == cacheKey || strings.HasPrefix(key, cacheKeyPrefix) {
			delete(archiveCache, key)
		}
	}
	archiveCacheMu.Unlock()
}
