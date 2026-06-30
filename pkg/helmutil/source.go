package helmutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	semver "github.com/blang/semver/v4"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"helm.sh/helm/v4/pkg/getter"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

const (
	ChartSourceRepository  = "repository"
	ChartSourceArtifactHub = "artifacthub"
	ChartSourceOCI         = "oci"

	artifactHubHelmPackageAPIURL = "https://artifacthub.io/api/v1/packages/helm/"
)

type ChartPackage struct {
	Version    string
	URL        string
	Repository *model.HelmRepository
}

type ChartSourceRef struct {
	Source         string
	RepositoryName string
	ChartName      string
	Version        string
	URL            string
}

func NormalizeChartSource(source, chartURL string) string {
	source = strings.TrimSpace(source)
	if source != "" {
		return source
	}
	if strings.HasPrefix(strings.TrimSpace(chartURL), "oci://") {
		return ChartSourceOCI
	}
	return ""
}

type artifactHubPackage struct {
	Version    string `json:"version"`
	ContentURL string `json:"content_url"`
}

func ResolveChartRepository(repositoryName, source string) (*model.HelmRepository, error) {
	if repositoryName == "" || source == ChartSourceArtifactHub || source == ChartSourceOCI {
		return nil, nil
	}
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return nil, err
	}
	return &repository, nil
}

func LatestChartPackage(ctx context.Context, source, repositoryName, chartName string) (ChartPackage, error) {
	switch source {
	case "", ChartSourceRepository:
		return latestRepositoryChartPackage(repositoryName, chartName)
	case ChartSourceArtifactHub:
		if !common.HelmArtifactHubEnabled {
			return ChartPackage{}, fmt.Errorf("Artifact Hub chart source is disabled")
		}
		return latestArtifactHubChartPackage(ctx, repositoryName, chartName)
	case ChartSourceOCI:
		return latestOCIChartPackage(repositoryName, chartName)
	default:
		return ChartPackage{}, fmt.Errorf("unsupported chart source")
	}
}

func ResolveChartPackage(ctx context.Context, ref ChartSourceRef) (ChartPackage, error) {
	switch ref.Source {
	case "", ChartSourceRepository:
		if strings.TrimSpace(ref.ChartName) == "" {
			repository, err := ResolveChartRepository(ref.RepositoryName, ref.Source)
			if err != nil {
				return ChartPackage{}, err
			}
			if err := ValidateChartURLSource(ref.URL, repository, ref.Source); err != nil {
				return ChartPackage{}, err
			}
			return ChartPackage{URL: strings.TrimSpace(ref.URL), Repository: repository}, nil
		}
		return repositoryChartPackage(ref.RepositoryName, ref.ChartName, ref.Version, ref.URL)
	case ChartSourceArtifactHub:
		if !common.HelmArtifactHubEnabled {
			return ChartPackage{}, fmt.Errorf("Artifact Hub chart source is disabled")
		}
		if strings.TrimSpace(ref.ChartName) == "" {
			return ChartPackage{}, fmt.Errorf("chartName is required for Artifact Hub charts")
		}
		return artifactHubChartPackage(ctx, ref.RepositoryName, ref.ChartName, ref.Version, ref.URL)
	case ChartSourceOCI:
		if strings.TrimSpace(ref.ChartName) == "" {
			return ChartPackage{}, fmt.Errorf("chartName is required for OCI charts")
		}
		return ociChartPackage(ref.RepositoryName, ref.ChartName, ref.Version, ref.URL)
	default:
		return ChartPackage{}, fmt.Errorf("unsupported chart source")
	}
}

func latestRepositoryChartPackage(repositoryName, chartName string) (ChartPackage, error) {
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return ChartPackage{}, err
	}
	indexFile, err := LoadRepositoryIndex(repository)
	if err != nil {
		return ChartPackage{}, err
	}
	versions := indexFile.Entries[chartName]
	if len(versions) == 0 {
		return ChartPackage{}, fmt.Errorf("chart not found")
	}
	latest := versions[0]
	for _, version := range versions[1:] {
		if CompareChartVersions(version.Version, latest.Version) > 0 {
			latest = version
		}
	}
	if len(latest.URLs) == 0 {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	return ChartPackage{
		Version:    latest.Version,
		URL:        ResolveURL(repository.URL, latest.URLs[0]),
		Repository: &repository,
	}, nil
}

func repositoryChartPackage(repositoryName, chartName, version, requestedURL string) (ChartPackage, error) {
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return ChartPackage{}, err
	}
	indexFile, err := LoadRepositoryIndex(repository)
	if err != nil {
		return ChartPackage{}, err
	}
	entry, err := indexFile.Get(chartName, version)
	if err != nil {
		return ChartPackage{}, err
	}
	if len(entry.URLs) == 0 {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	resolvedURL := ResolveURL(repository.URL, entry.URLs[0])
	if strings.TrimSpace(requestedURL) != "" && strings.TrimSpace(requestedURL) != resolvedURL {
		return ChartPackage{}, fmt.Errorf("chartUrl does not match repository chart package")
	}
	return ChartPackage{
		Version:    entry.Version,
		URL:        resolvedURL,
		Repository: &repository,
	}, nil
}

func LoadRepositoryIndex(repository model.HelmRepository) (*repo.IndexFile, error) {
	entry := &repo.Entry{
		Name:     repository.Name,
		URL:      repository.URL,
		Username: repository.Username,
		Password: string(repository.Password),
	}
	chartRepository, err := repo.NewChartRepository(entry, getter.Getters())
	if err != nil {
		return nil, err
	}
	cacheDir, err := os.MkdirTemp("", "kite-helm-repo-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()
	chartRepository.CachePath = cacheDir

	indexPath, err := chartRepository.DownloadIndexFile()
	if err != nil {
		return nil, err
	}
	return repo.LoadIndexFile(indexPath)
}

func latestOCIChartPackage(repositoryName, chartName string) (ChartPackage, error) {
	ref, err := LatestOCIChartVersion(repositoryName, chartName)
	if err != nil {
		return ChartPackage{}, err
	}
	return ChartPackage{
		Version: ref.Version.Version,
		URL:     ref.ChartURL,
	}, nil
}

func ociChartPackage(repositoryName, chartName, version, requestedURL string) (ChartPackage, error) {
	ref, err := FindOCIChartVersion(repositoryName, chartName, version)
	if err != nil {
		return ChartPackage{}, err
	}
	if strings.TrimSpace(requestedURL) != "" && strings.TrimSpace(requestedURL) != ref.ChartURL {
		return ChartPackage{}, fmt.Errorf("chartUrl does not match OCI chart package")
	}
	return ChartPackage{
		Version: ref.Version.Version,
		URL:     ref.ChartURL,
	}, nil
}

func latestArtifactHubChartPackage(ctx context.Context, repositoryName, chartName string) (ChartPackage, error) {
	packageURL := artifactHubHelmPackageAPIURL + url.PathEscape(repositoryName) + "/" + url.PathEscape(chartName)
	return fetchArtifactHubChartPackage(ctx, packageURL, "")
}

func artifactHubChartPackage(ctx context.Context, repositoryName, chartName, version, requestedURL string) (ChartPackage, error) {
	packageURL := artifactHubHelmPackageAPIURL + url.PathEscape(repositoryName) + "/" + url.PathEscape(chartName)
	if strings.TrimSpace(version) != "" {
		packageURL += "/" + url.PathEscape(version)
	}
	return fetchArtifactHubChartPackage(ctx, packageURL, requestedURL)
}

func fetchArtifactHubChartPackage(ctx context.Context, packageURL, requestedURL string) (ChartPackage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return ChartPackage{}, err
	}
	req.Header.Set("User-Agent", "kite")

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ChartPackage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ChartPackage{}, fmt.Errorf("artifact hub request failed: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChartPackage{}, err
	}
	var pkg artifactHubPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ChartPackage{}, err
	}
	if strings.TrimSpace(pkg.ContentURL) == "" {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	if strings.TrimSpace(requestedURL) != "" && strings.TrimSpace(requestedURL) != pkg.ContentURL {
		return ChartPackage{}, fmt.Errorf("chartUrl does not match Artifact Hub chart package")
	}
	return ChartPackage{
		Version: pkg.Version,
		URL:     pkg.ContentURL,
	}, nil
}

func IsChartVersionNewer(next, current string) bool {
	return CompareChartVersions(next, current) > 0
}

func CompareChartVersions(a, b string) int {
	parsedA, errA := semver.ParseTolerant(a)
	parsedB, errB := semver.ParseTolerant(b)
	if errA == nil && errB == nil {
		return parsedA.Compare(parsedB)
	}
	if errA == nil {
		return 1
	}
	if errB == nil {
		return -1
	}
	return strings.Compare(a, b)
}
