package helmutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/model"
	"helm.sh/helm/v4/pkg/getter"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

func TestLoadRepositoryArchiveFromRepository(t *testing.T) {
	repository := model.HelmRepository{
		Name: "kite",
		URL:  "https://kite-org.github.io/kite/",
	}
	chartRepository, err := repo.NewChartRepository(&repo.Entry{
		Name: repository.Name,
		URL:  repository.URL,
	}, getter.Getters())
	require.NoError(t, err)
	chartRepository.CachePath = t.TempDir()

	indexPath, err := chartRepository.DownloadIndexFile()
	require.NoError(t, err)
	indexFile, err := repo.LoadIndexFile(indexPath)
	require.NoError(t, err)

	entries := indexFile.Entries["kite"]
	require.NotEmpty(t, entries)

	loadedChart, err := LoadRepositoryArchive(repository, entries[0])
	require.NoError(t, err)
	require.Equal(t, "kite", loadedChart.Metadata.Name)
	require.Equal(t, entries[0].Version, loadedChart.Metadata.Version)
}

func TestLoadArchiveFromOCIRepository(t *testing.T) {
	loadedChart, err := LoadArchive("oci://ghcr.io/kite-org/charts/kite", nil)
	require.NoError(t, err)
	require.Equal(t, "kite", loadedChart.Metadata.Name)
	require.NotEmpty(t, loadedChart.Metadata.Version)
}

func TestRegistryOptionsForArchiveUsesDiscoveryOptions(t *testing.T) {
	t.Setenv(ociRegistryBaseEnv, "oci://registry.local/charts")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryInsecureTLSEnv, "true")
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")

	options, err := registryOptionsForArchive("oci://registry.local/charts/postgres:12.0.0", nil, false, nil)
	require.NoError(t, err)
	require.True(t, options.PlainHTTP)
	require.True(t, options.InsecureSkipTLSVerify)
	require.Equal(t, "admin", options.Username)
	require.Equal(t, "secret", options.Password)

	options, err = registryOptionsForArchive("oci://registry.local/charts2/postgres:12.0.0", nil, false, nil)
	require.NoError(t, err)
	require.Empty(t, options.Username)
	require.Empty(t, options.Password)
}

func TestRegistryOptionsForArchivePrefersExplicitOptions(t *testing.T) {
	t.Setenv(ociRegistryBaseEnv, "oci://registry.local/charts")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryUsernameEnv, "catalog-user")
	t.Setenv(ociRegistryPasswordEnv, "catalog-secret")

	explicit := OCIRegistryOptions{
		Username: "explicit-user",
		Password: "explicit-secret",
	}
	options, err := registryOptionsForArchive("oci://registry.local/charts/postgres:12.0.0", nil, false, &explicit)
	require.NoError(t, err)
	require.False(t, options.PlainHTTP)
	require.Equal(t, "explicit-user", options.Username)
	require.Equal(t, "explicit-secret", options.Password)
}

func TestNewOCIRegistryClientAllowsPlainHTTPWithInsecureTokenRealm(t *testing.T) {
	client, err := newOCIRegistryClient(OCIRegistryOptions{
		PlainHTTP:             true,
		InsecureSkipTLSVerify: true,
		Username:              "admin",
		Password:              "secret",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
}
