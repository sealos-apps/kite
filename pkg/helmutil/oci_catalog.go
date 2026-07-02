package helmutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
)

const (
	ociRegistryBaseEnv                  = "KITE_HELM_OCI_REGISTRY_BASE"
	ociRepositoryNameEnv                = "KITE_HELM_OCI_REPOSITORY_NAME"
	ociRegistryPlainHTTPEnv             = "KITE_HELM_OCI_REGISTRY_PLAIN_HTTP"
	ociRegistryInsecureTLSEnv           = "KITE_HELM_OCI_REGISTRY_INSECURE_SKIP_TLS_VERIFY"
	ociRegistryUsernameEnv              = "KITE_HELM_OCI_REGISTRY_USERNAME"
	ociRegistryPasswordEnv              = "KITE_HELM_OCI_REGISTRY_PASSWORD"
	ociRegistryCAFileEnv                = "KITE_HELM_OCI_REGISTRY_CA_FILE"
	ociDiscoveryPageSizeEnv             = "KITE_HELM_OCI_DISCOVERY_PAGE_SIZE"
	ociDiscoveryMaxRepositoriesEnv      = "KITE_HELM_OCI_DISCOVERY_MAX_REPOSITORIES"
	ociDiscoveryMaxTagsPerRepositoryEnv = "KITE_HELM_OCI_DISCOVERY_MAX_TAGS_PER_REPOSITORY"

	defaultOCIRepositoryName                = "offline"
	defaultOCIDiscoveryPageSize             = 100
	defaultOCIDiscoveryMaxRepositories      = 1000
	defaultOCIDiscoveryMaxTagsPerRepository = 200
	ociDiscoveryCacheTTL                    = 5 * time.Minute

	helmOCIConfigMediaType = "application/vnd.cncf.helm.config.v1+json"
	helmOCIChartLayerType  = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
)

type OCIChartCatalog struct {
	Repositories []OCIChartRepository `json:"repositories,omitempty"`
}

type OCIChartRepository struct {
	Name     string             `json:"name"`
	URL      string             `json:"url"`
	Charts   []OCIChart         `json:"charts"`
	Registry OCIRegistryOptions `json:"-"`
}

type OCIChart struct {
	Name        string              `json:"name"`
	Version     string              `json:"version,omitempty"`
	AppVersion  string              `json:"appVersion,omitempty"`
	KubeVersion string              `json:"kubeVersion,omitempty"`
	Description string              `json:"description,omitempty"`
	Icon        string              `json:"icon,omitempty"`
	Home        string              `json:"home,omitempty"`
	Sources     []string            `json:"sources,omitempty"`
	Keywords    []string            `json:"keywords,omitempty"`
	Maintainers []*chart.Maintainer `json:"maintainers,omitempty"`
	Deprecated  bool                `json:"deprecated,omitempty"`
	UpdatedAt   string              `json:"updatedAt,omitempty"`
	ChartURL    string              `json:"chartUrl,omitempty"`
	Versions    []OCIChartVersion   `json:"versions,omitempty"`
}

type OCIChartVersion struct {
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion,omitempty"`
	KubeVersion string `json:"kubeVersion,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Home        string `json:"home,omitempty"`
	ChartURL    string `json:"chartUrl,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type OCIChartVersionRef struct {
	RepositoryName string
	RepositoryURL  string
	Registry       OCIRegistryOptions `json:"-"`
	Chart          OCIChart
	Version        OCIChartVersion
	ChartURL       string
}

type OCIRegistryOptions struct {
	PlainHTTP             bool   `json:"-"`
	InsecureSkipTLSVerify bool   `json:"-"`
	CAFile                string `json:"-"`
	Username              string `json:"-"`
	Password              string `json:"-"`
}

type OCIRegistryDiscoveryConfig struct {
	Enabled              bool
	BaseURL              string
	RepositoryName       string
	RepositoryPrefix     string
	RegistryHost         string
	RegistryOptions      OCIRegistryOptions
	PageSize             int
	MaxRepositories      int
	MaxTagsPerRepository int
}

type cachedOCIDiscovery struct {
	key       string
	refs      []OCIChartVersionRef
	expiresAt time.Time
	loaded    bool
}

type registryCatalogResponse struct {
	Repositories []string `json:"repositories"`
}

type registryTagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type ociManifest struct {
	Config struct {
		MediaType string `json:"mediaType"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
	} `json:"layers"`
}

type ociDiscoveryClient struct {
	config OCIRegistryDiscoveryConfig
	client *http.Client
}

var (
	ociDiscoveryCacheMu sync.Mutex
	ociDiscoveryCache   cachedOCIDiscovery
)

func LoadOCIChartCatalog() (OCIChartCatalog, error) {
	refs, err := ListOCIChartVersions()
	if err != nil {
		return OCIChartCatalog{}, err
	}
	if len(refs) == 0 {
		return OCIChartCatalog{}, nil
	}

	repository := OCIChartRepository{
		Name:     refs[0].RepositoryName,
		URL:      refs[0].RepositoryURL,
		Registry: refs[0].Registry,
	}
	chartIndexes := map[string]int{}
	for _, ref := range refs {
		index, ok := chartIndexes[ref.Chart.Name]
		if !ok {
			repository.Charts = append(repository.Charts, ref.Chart)
			index = len(repository.Charts) - 1
			chartIndexes[ref.Chart.Name] = index
		}
		repository.Charts[index].Versions = append(repository.Charts[index].Versions, ref.Version)
	}
	return OCIChartCatalog{Repositories: []OCIChartRepository{repository}}, nil
}

func ListOCIChartVersions() ([]OCIChartVersionRef, error) {
	config, err := loadOCIRegistryDiscoveryConfig()
	if err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}

	cacheKey := ociDiscoveryCacheKey(config)
	now := time.Now()
	ociDiscoveryCacheMu.Lock()
	if ociDiscoveryCache.loaded && ociDiscoveryCache.key == cacheKey && now.Before(ociDiscoveryCache.expiresAt) {
		refs := cloneOCIChartVersionRefs(ociDiscoveryCache.refs)
		ociDiscoveryCacheMu.Unlock()
		return refs, nil
	}
	ociDiscoveryCacheMu.Unlock()

	refs, err := discoverOCIChartVersions(config)
	if err != nil {
		ociDiscoveryCacheMu.Lock()
		if ociDiscoveryCache.loaded && ociDiscoveryCache.key == cacheKey {
			refs := cloneOCIChartVersionRefs(ociDiscoveryCache.refs)
			ociDiscoveryCacheMu.Unlock()
			return refs, nil
		}
		ociDiscoveryCacheMu.Unlock()
		return nil, err
	}

	ociDiscoveryCacheMu.Lock()
	ociDiscoveryCache = cachedOCIDiscovery{
		key:       cacheKey,
		refs:      cloneOCIChartVersionRefs(refs),
		expiresAt: now.Add(ociDiscoveryCacheTTL),
		loaded:    true,
	}
	ociDiscoveryCacheMu.Unlock()

	return refs, nil
}

func ClearOCIChartDiscoveryCache() {
	ociDiscoveryCacheMu.Lock()
	ociDiscoveryCache = cachedOCIDiscovery{}
	ociDiscoveryCacheMu.Unlock()
}

func LatestOCIChartVersion(repositoryName, chartName string) (OCIChartVersionRef, error) {
	refs, err := matchingOCIChartVersions(repositoryName, chartName)
	if err != nil {
		return OCIChartVersionRef{}, err
	}
	if len(refs) == 0 {
		return OCIChartVersionRef{}, fmt.Errorf("chart not found")
	}
	latest := refs[0]
	for _, ref := range refs[1:] {
		if CompareChartVersions(ref.Version.Version, latest.Version.Version) > 0 {
			latest = ref
		}
	}
	return latest, nil
}

func FindOCIChartVersion(repositoryName, chartName, version string) (OCIChartVersionRef, error) {
	if strings.TrimSpace(version) == "" {
		return LatestOCIChartVersion(repositoryName, chartName)
	}
	refs, err := matchingOCIChartVersions(repositoryName, chartName)
	if err != nil {
		return OCIChartVersionRef{}, err
	}
	for _, ref := range refs {
		if ref.Version.Version == version {
			return ref, nil
		}
	}
	return OCIChartVersionRef{}, fmt.Errorf("chart version not found")
}

func OCIChartVersionURL(chartURL, version string) string {
	chartURL = strings.TrimSpace(chartURL)
	if chartURL == "" || version == "" || ociURLHasReference(chartURL) {
		return chartURL
	}
	return chartURL + ":" + ociTagFromChartVersion(version)
}

func OCIChartUpdatedAt(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func OCIRegistryOptionsForChartURL(chartURL string) (OCIRegistryOptions, bool, error) {
	config, err := loadOCIRegistryDiscoveryConfig()
	if err != nil {
		return OCIRegistryOptions{}, false, err
	}
	if !config.Enabled {
		return OCIRegistryOptions{}, false, nil
	}
	chartURL = strings.TrimRight(strings.TrimSpace(chartURL), "/")
	if chartURL == "" {
		return OCIRegistryOptions{}, false, nil
	}
	chartURLWithoutReference := ociURLWithoutReference(chartURL)
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if chartURLWithoutReference == baseURL || strings.HasPrefix(chartURLWithoutReference, baseURL+"/") {
		return config.RegistryOptions, true, nil
	}
	return OCIRegistryOptions{}, false, nil
}

func matchingOCIChartVersions(repositoryName, chartName string) ([]OCIChartVersionRef, error) {
	repositoryName = strings.TrimSpace(repositoryName)
	chartName = strings.TrimSpace(chartName)
	if chartName == "" {
		return nil, fmt.Errorf("chartName is required for OCI charts")
	}
	refs, err := ListOCIChartVersions()
	if err != nil {
		return nil, err
	}
	matches := []OCIChartVersionRef{}
	for _, ref := range refs {
		if repositoryName != "" && ref.RepositoryName != repositoryName {
			continue
		}
		if ref.Chart.Name == chartName {
			matches = append(matches, ref)
		}
	}
	return matches, nil
}

func discoverOCIChartVersions(config OCIRegistryDiscoveryConfig) ([]OCIChartVersionRef, error) {
	client, err := newOCIDiscoveryClient(config)
	if err != nil {
		return nil, err
	}
	repositories, err := client.listRepositories()
	if err != nil {
		return nil, err
	}

	repository := OCIChartRepository{
		Name:     config.RepositoryName,
		URL:      config.BaseURL,
		Registry: config.RegistryOptions,
	}
	refs := []OCIChartVersionRef{}
	for _, repositoryPath := range repositories {
		chartName, ok := chartNameFromRepositoryPath(config.RepositoryPrefix, repositoryPath)
		if !ok {
			continue
		}
		tags, err := client.listTags(repositoryPath)
		if err != nil {
			return nil, err
		}
		chartURL := "oci://" + config.RegistryHost + "/" + strings.Trim(repositoryPath, "/")
		chart := OCIChart{
			Name:     chartName,
			ChartURL: chartURL,
		}
		for _, tag := range tags {
			isHelmChart, err := client.isHelmChartTag(repositoryPath, tag)
			if err != nil {
				return nil, err
			}
			if !isHelmChart {
				continue
			}
			version := OCIChartVersion{
				Version: chartVersionFromOCITag(tag),
			}
			refs = append(refs, newOCIChartVersionRef(repository, chart, version))
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].RepositoryName != refs[j].RepositoryName {
			return refs[i].RepositoryName < refs[j].RepositoryName
		}
		if refs[i].Chart.Name != refs[j].Chart.Name {
			return refs[i].Chart.Name < refs[j].Chart.Name
		}
		return CompareChartVersions(refs[i].Version.Version, refs[j].Version.Version) > 0
	})
	return refs, nil
}

func newOCIChartVersionRef(repository OCIChartRepository, chart OCIChart, version OCIChartVersion) OCIChartVersionRef {
	chartURL := strings.TrimSpace(version.ChartURL)
	if chartURL == "" {
		chartURL = chart.ChartURL
	}
	return OCIChartVersionRef{
		RepositoryName: repository.Name,
		RepositoryURL:  repository.URL,
		Registry:       repository.Registry,
		Chart:          chart,
		Version:        version,
		ChartURL:       OCIChartVersionURL(chartURL, version.Version),
	}
}

func loadOCIRegistryDiscoveryConfig() (OCIRegistryDiscoveryConfig, error) {
	registryOptions, err := loadOCIRegistryOptions()
	if err != nil {
		return OCIRegistryDiscoveryConfig{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(ociRegistryBaseEnv)), "/")
	if baseURL == "" {
		return OCIRegistryDiscoveryConfig{RegistryOptions: registryOptions}, nil
	}
	if err := validateOCIChartURL(baseURL); err != nil {
		return OCIRegistryDiscoveryConfig{}, fmt.Errorf("invalid %s: %w", ociRegistryBaseEnv, err)
	}
	if ociURLHasReference(baseURL) {
		return OCIRegistryDiscoveryConfig{}, fmt.Errorf("%s must not include a tag or digest", ociRegistryBaseEnv)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return OCIRegistryDiscoveryConfig{}, err
	}
	repositoryPrefix := strings.Trim(parsed.Path, "/")
	if repositoryPrefix == "" {
		return OCIRegistryDiscoveryConfig{}, fmt.Errorf("%s must include a repository prefix", ociRegistryBaseEnv)
	}
	repositoryName := strings.TrimSpace(os.Getenv(ociRepositoryNameEnv))
	if repositoryName == "" {
		repositoryName = defaultOCIRepositoryName
	}
	pageSize, err := parsePositiveIntEnv(ociDiscoveryPageSizeEnv, defaultOCIDiscoveryPageSize)
	if err != nil {
		return OCIRegistryDiscoveryConfig{}, err
	}
	maxRepositories, err := parsePositiveIntEnv(ociDiscoveryMaxRepositoriesEnv, defaultOCIDiscoveryMaxRepositories)
	if err != nil {
		return OCIRegistryDiscoveryConfig{}, err
	}
	maxTagsPerRepository, err := parsePositiveIntEnv(ociDiscoveryMaxTagsPerRepositoryEnv, defaultOCIDiscoveryMaxTagsPerRepository)
	if err != nil {
		return OCIRegistryDiscoveryConfig{}, err
	}
	return OCIRegistryDiscoveryConfig{
		Enabled:              true,
		BaseURL:              baseURL,
		RepositoryName:       repositoryName,
		RepositoryPrefix:     repositoryPrefix,
		RegistryHost:         parsed.Host,
		RegistryOptions:      registryOptions,
		PageSize:             pageSize,
		MaxRepositories:      maxRepositories,
		MaxTagsPerRepository: maxTagsPerRepository,
	}, nil
}

func loadOCIRegistryOptions() (OCIRegistryOptions, error) {
	plainHTTP, err := parseOptionalBoolEnv(ociRegistryPlainHTTPEnv)
	if err != nil {
		return OCIRegistryOptions{}, err
	}
	insecureSkipTLSVerify, err := parseOptionalBoolEnv(ociRegistryInsecureTLSEnv)
	if err != nil {
		return OCIRegistryOptions{}, err
	}
	return OCIRegistryOptions{
		PlainHTTP:             plainHTTP,
		InsecureSkipTLSVerify: insecureSkipTLSVerify,
		CAFile:                strings.TrimSpace(os.Getenv(ociRegistryCAFileEnv)),
		Username:              strings.TrimSpace(os.Getenv(ociRegistryUsernameEnv)),
		Password:              os.Getenv(ociRegistryPasswordEnv),
	}, nil
}

func parseOptionalBoolEnv(name string) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}

func parsePositiveIntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s: must be a positive integer", name)
	}
	return parsed, nil
}

func newOCIDiscoveryClient(config OCIRegistryDiscoveryConfig) (*ociDiscoveryClient, error) {
	httpClient := http.DefaultClient
	if !config.RegistryOptions.PlainHTTP && (config.RegistryOptions.InsecureSkipTLSVerify || strings.TrimSpace(config.RegistryOptions.CAFile) != "") {
		tlsConfig, err := newOCITLSConfig(config.RegistryOptions)
		if err != nil {
			return nil, err
		}
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				Proxy:           http.ProxyFromEnvironment,
			},
		}
	}
	return &ociDiscoveryClient{
		config: config,
		client: httpClient,
	}, nil
}

func (c *ociDiscoveryClient) listRepositories() ([]string, error) {
	repositories := []string{}
	last := ""
	scannedRepositories := 0
	for scannedRepositories < c.config.MaxRepositories {
		var response registryCatalogResponse
		query := url.Values{}
		query.Set("n", strconv.Itoa(c.config.PageSize))
		if last != "" {
			query.Set("last", last)
		}
		if err := c.getJSON([]string{"_catalog"}, query, &response); err != nil {
			return nil, err
		}
		if len(response.Repositories) == 0 {
			break
		}
		for _, repository := range response.Repositories {
			scannedRepositories++
			repository = strings.Trim(repository, "/")
			if _, ok := chartNameFromRepositoryPath(c.config.RepositoryPrefix, repository); !ok {
				if scannedRepositories >= c.config.MaxRepositories {
					break
				}
				continue
			}
			repositories = append(repositories, repository)
			if scannedRepositories >= c.config.MaxRepositories {
				break
			}
		}
		nextLast := response.Repositories[len(response.Repositories)-1]
		if len(response.Repositories) < c.config.PageSize || nextLast == last {
			break
		}
		last = nextLast
	}
	return repositories, nil
}

func (c *ociDiscoveryClient) listTags(repositoryPath string) ([]string, error) {
	tags := []string{}
	last := ""
	for len(tags) < c.config.MaxTagsPerRepository {
		var response registryTagsResponse
		query := url.Values{}
		query.Set("n", strconv.Itoa(c.config.PageSize))
		if last != "" {
			query.Set("last", last)
		}
		if err := c.getJSON([]string{repositoryPath, "tags", "list"}, query, &response); err != nil {
			return nil, err
		}
		if len(response.Tags) == 0 {
			break
		}
		for _, tag := range response.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			tags = append(tags, tag)
			if len(tags) >= c.config.MaxTagsPerRepository {
				break
			}
		}
		nextLast := response.Tags[len(response.Tags)-1]
		if len(response.Tags) < c.config.PageSize || nextLast == last {
			break
		}
		last = nextLast
	}
	return tags, nil
}

func (c *ociDiscoveryClient) isHelmChartTag(repositoryPath, tag string) (bool, error) {
	var manifest ociManifest
	statusCode, err := c.getJSONWithStatus([]string{repositoryPath, "manifests", tag}, nil, &manifest, strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	if statusCode == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if manifest.Config.MediaType == helmOCIConfigMediaType {
		return true, nil
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType == helmOCIChartLayerType {
			return true, nil
		}
	}
	return false, nil
}

func (c *ociDiscoveryClient) getJSON(parts []string, query url.Values, target any) error {
	_, err := c.getJSONWithStatus(parts, query, target, "application/json")
	return err
}

func (c *ociDiscoveryClient) getJSONWithStatus(parts []string, query url.Values, target any, accept string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, c.registryAPIURL(parts, query), nil)
	if err != nil {
		return 0, err
	}
	if c.config.RegistryOptions.Username != "" {
		req.SetBasicAuth(c.config.RegistryOptions.Username, c.config.RegistryOptions.Password)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("OCI registry request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func (c *ociDiscoveryClient) registryAPIURL(parts []string, query url.Values) string {
	scheme := "https"
	if c.config.RegistryOptions.PlainHTTP {
		scheme = "http"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   c.config.RegistryHost,
		Path:   path.Join(append([]string{"/v2"}, parts...)...),
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func chartNameFromRepositoryPath(repositoryPrefix, repositoryPath string) (string, bool) {
	repositoryPrefix = strings.Trim(repositoryPrefix, "/")
	repositoryPath = strings.Trim(repositoryPath, "/")
	if repositoryPrefix == "" || !strings.HasPrefix(repositoryPath, repositoryPrefix+"/") {
		return "", false
	}
	chartName := strings.TrimPrefix(repositoryPath, repositoryPrefix+"/")
	if chartName == "" || strings.Contains(chartName, "/") {
		return "", false
	}
	return chartName, true
}

func validateOCIChartURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("OCI URL must be absolute")
	}
	if strings.ToLower(parsed.Scheme) != "oci" {
		return fmt.Errorf("OCI URL must use oci scheme")
	}
	if parsed.User != nil {
		return fmt.Errorf("OCI URL must not include credentials")
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("OCI URL must not include query parameters")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("OCI URL must not include fragments")
	}
	return nil
}

func ociURLHasReference(rawURL string) bool {
	parts := ociURLReferenceParts(rawURL)
	return parts.hasTag || parts.hasDigest
}

func validateOCIChartVersionURL(rawURL, version string) error {
	parts := ociURLReferenceParts(rawURL)
	if !parts.hasTag {
		if parts.hasDigest {
			return fmt.Errorf("OCI chartUrl digest references must include the version tag")
		}
		return nil
	}
	expectedTag := ociTagFromChartVersion(version)
	if parts.tag != expectedTag {
		return fmt.Errorf("OCI chartUrl tag %q does not match version %q", parts.tag, version)
	}
	return nil
}

type ociURLReference struct {
	tag       string
	hasTag    bool
	hasDigest bool
}

func ociURLReferenceParts(rawURL string) ociURLReference {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ociURLReference{}
	}
	base := path.Base(parsed.Path)
	namePart, _, hasDigest := strings.Cut(base, "@")
	tag := ""
	hasTag := false
	if index := strings.LastIndex(namePart, ":"); index >= 0 {
		tag = namePart[index+1:]
		hasTag = tag != ""
	}
	return ociURLReference{
		tag:       tag,
		hasTag:    hasTag,
		hasDigest: hasDigest,
	}
}

func ociURLWithoutReference(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(rawURL), "/")
	}
	base := path.Base(parsed.Path)
	namePart, _, _ := strings.Cut(base, "@")
	if index := strings.LastIndex(namePart, ":"); index >= 0 {
		namePart = namePart[:index]
	}
	if namePart != base {
		parsed.Path = path.Join(path.Dir(parsed.Path), namePart)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func ociTagFromChartVersion(version string) string {
	return strings.ReplaceAll(version, "+", "_")
}

func chartVersionFromOCITag(tag string) string {
	return strings.ReplaceAll(tag, "_", "+")
}

func cloneOCIChartVersionRefs(refs []OCIChartVersionRef) []OCIChartVersionRef {
	return append([]OCIChartVersionRef(nil), refs...)
}

func ociDiscoveryCacheKey(config OCIRegistryDiscoveryConfig) string {
	return strings.Join([]string{
		config.BaseURL,
		config.RepositoryName,
		config.RepositoryPrefix,
		config.RegistryHost,
		strconv.FormatBool(config.RegistryOptions.PlainHTTP),
		strconv.FormatBool(config.RegistryOptions.InsecureSkipTLSVerify),
		config.RegistryOptions.CAFile,
		config.RegistryOptions.Username,
		config.RegistryOptions.Password,
		strconv.Itoa(config.PageSize),
		strconv.Itoa(config.MaxRepositories),
		strconv.Itoa(config.MaxTagsPerRepository),
	}, "\x00")
}
