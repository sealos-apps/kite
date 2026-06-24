package helmutil

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"sigs.k8s.io/yaml"
)

const (
	ociCatalogEnv               = "KITE_HELM_OCI_CATALOG"
	ociCatalogFileEnv           = "KITE_HELM_OCI_CATALOG_FILE"
	ociCatalogBaseURLEnv        = "KITE_HELM_OCI_CATALOG_BASE"
	ociCatalogRepositoryNameEnv = "KITE_HELM_OCI_CATALOG_REPOSITORY_NAME"
	ociRegistryPlainHTTPEnv     = "KITE_HELM_OCI_REGISTRY_PLAIN_HTTP"
	ociRegistryInsecureTLSEnv   = "KITE_HELM_OCI_REGISTRY_INSECURE_SKIP_TLS_VERIFY"
	ociRegistryUsernameEnv      = "KITE_HELM_OCI_REGISTRY_USERNAME"
	ociRegistryPasswordEnv      = "KITE_HELM_OCI_REGISTRY_PASSWORD"
	ociRegistryCAFileEnv        = "KITE_HELM_OCI_REGISTRY_CA_FILE"
	defaultOCIRepositoryName    = "offline"
)

type OCIChartCatalog struct {
	Repositories []OCIChartRepository `json:"repositories,omitempty"`
	Charts       []OCIChart           `json:"charts,omitempty"`
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

func LoadOCIChartCatalog() (OCIChartCatalog, error) {
	registryOptions, err := loadOCIRegistryOptions()
	if err != nil {
		return OCIChartCatalog{}, err
	}

	data := strings.TrimSpace(os.Getenv(ociCatalogEnv))
	if file := strings.TrimSpace(os.Getenv(ociCatalogFileEnv)); file != "" {
		content, err := os.ReadFile(file)
		if err != nil {
			return OCIChartCatalog{}, err
		}
		data = strings.TrimSpace(string(content))
	}

	var catalog OCIChartCatalog
	if data != "" {
		if err := yaml.Unmarshal([]byte(data), &catalog); err != nil {
			return OCIChartCatalog{}, err
		}
	}
	if len(catalog.Charts) > 0 {
		baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(ociCatalogBaseURLEnv)), "/")
		if baseURL == "" {
			return OCIChartCatalog{}, fmt.Errorf("%s is required when top-level OCI charts are configured", ociCatalogBaseURLEnv)
		}
		repositoryName := strings.TrimSpace(os.Getenv(ociCatalogRepositoryNameEnv))
		if repositoryName == "" {
			repositoryName = defaultOCIRepositoryName
		}
		catalog.Repositories = append(catalog.Repositories, OCIChartRepository{
			Name:   repositoryName,
			URL:    baseURL,
			Charts: catalog.Charts,
		})
		catalog.Charts = nil
	}

	if err := normalizeOCIChartCatalog(&catalog); err != nil {
		return OCIChartCatalog{}, err
	}
	for i := range catalog.Repositories {
		catalog.Repositories[i].Registry = registryOptions
	}
	return catalog, nil
}

func ListOCIChartVersions() ([]OCIChartVersionRef, error) {
	catalog, err := LoadOCIChartCatalog()
	if err != nil {
		return nil, err
	}
	refs := []OCIChartVersionRef{}
	for _, repository := range catalog.Repositories {
		for _, chart := range repository.Charts {
			for _, version := range ociChartVersions(chart) {
				refs = append(refs, newOCIChartVersionRef(repository, chart, version))
			}
		}
	}
	return refs, nil
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
	catalog, err := LoadOCIChartCatalog()
	if err != nil {
		return OCIRegistryOptions{}, false, err
	}
	chartURL = strings.TrimRight(strings.TrimSpace(chartURL), "/")
	if chartURL == "" {
		return OCIRegistryOptions{}, false, nil
	}
	chartURLWithoutReference := ociURLWithoutReference(chartURL)
	for _, repository := range catalog.Repositories {
		for _, chart := range repository.Charts {
			for _, version := range ociChartVersions(chart) {
				ref := newOCIChartVersionRef(repository, chart, version)
				if ref.ChartURL == chartURL || ociURLWithoutReference(ref.ChartURL) == chartURLWithoutReference {
					return ref.Registry, true, nil
				}
			}
		}
		repositoryURL := strings.TrimRight(repository.URL, "/")
		if chartURLWithoutReference == repositoryURL || strings.HasPrefix(chartURLWithoutReference, repositoryURL+"/") {
			return repository.Registry, true, nil
		}
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

func normalizeOCIChartCatalog(catalog *OCIChartCatalog) error {
	for i := range catalog.Repositories {
		repository := &catalog.Repositories[i]
		repository.Name = strings.TrimSpace(repository.Name)
		if repository.Name == "" {
			return fmt.Errorf("OCI repository name is required")
		}
		repository.URL = strings.TrimRight(strings.TrimSpace(repository.URL), "/")
		if err := validateOCIChartURL(repository.URL); err != nil {
			return fmt.Errorf("invalid OCI repository %s: %w", repository.Name, err)
		}
		for j := range repository.Charts {
			chart := &repository.Charts[j]
			chart.Name = strings.TrimSpace(chart.Name)
			if chart.Name == "" {
				return fmt.Errorf("OCI chart name is required")
			}
			chart.Version = strings.TrimSpace(chart.Version)
			chart.ChartURL = strings.TrimRight(strings.TrimSpace(chart.ChartURL), "/")
			if chart.ChartURL == "" {
				chart.ChartURL = repository.URL + "/" + chart.Name
			}
			if err := validateOCIChartURL(chart.ChartURL); err != nil {
				return fmt.Errorf("invalid OCI chart %s/%s: %w", repository.Name, chart.Name, err)
			}
			for k := range chart.Versions {
				version := &chart.Versions[k]
				version.Version = strings.TrimSpace(version.Version)
				if version.Version == "" {
					return fmt.Errorf("OCI chart %s/%s version is required", repository.Name, chart.Name)
				}
				version.ChartURL = strings.TrimRight(strings.TrimSpace(version.ChartURL), "/")
				if version.ChartURL != "" {
					if err := validateOCIChartURL(version.ChartURL); err != nil {
						return fmt.Errorf("invalid OCI chart %s/%s version %s: %w", repository.Name, chart.Name, version.Version, err)
					}
					if err := validateOCIChartVersionURL(version.ChartURL, version.Version); err != nil {
						return fmt.Errorf("invalid OCI chart %s/%s version %s: %w", repository.Name, chart.Name, version.Version, err)
					}
				}
			}
			if ociURLHasReference(chart.ChartURL) {
				versions := ociChartVersions(*chart)
				if len(versions) != 1 {
					return fmt.Errorf("OCI chart %s/%s chartUrl with tag or digest requires exactly one version", repository.Name, chart.Name)
				}
				if err := validateOCIChartVersionURL(chart.ChartURL, versions[0].Version); err != nil {
					return fmt.Errorf("invalid OCI chart %s/%s: %w", repository.Name, chart.Name, err)
				}
			}
		}
	}
	return nil
}

func ociChartVersions(chart OCIChart) []OCIChartVersion {
	if len(chart.Versions) > 0 {
		return chart.Versions
	}
	if strings.TrimSpace(chart.Version) == "" {
		return nil
	}
	return []OCIChartVersion{{
		Version:     chart.Version,
		AppVersion:  chart.AppVersion,
		KubeVersion: chart.KubeVersion,
		Description: chart.Description,
		Icon:        chart.Icon,
		Home:        chart.Home,
		ChartURL:    chart.ChartURL,
		UpdatedAt:   chart.UpdatedAt,
	}}
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
