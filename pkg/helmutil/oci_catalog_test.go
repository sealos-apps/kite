package helmutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOCIChartCatalogFromRepositories(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: redis
        description: Redis chart
        versions:
          - version: 1.2.3
            appVersion: "7.2"
          - version: 1.3.0
            chartUrl: oci://registry.local/custom/redis
`)

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Len(t, refs, 2)

	latest, err := LatestOCIChartVersion("offline", "redis")
	require.NoError(t, err)
	require.Equal(t, "1.3.0", latest.Version.Version)
	require.Equal(t, "oci://registry.local/custom/redis:1.3.0", latest.ChartURL)

	version, err := FindOCIChartVersion("offline", "redis", "1.2.3")
	require.NoError(t, err)
	require.Equal(t, "Redis chart", version.Chart.Description)
	require.Equal(t, "7.2", version.Version.AppVersion)
	require.Equal(t, "oci://registry.local/charts/redis:1.2.3", version.ChartURL)
}

func TestLoadOCIChartCatalogFromTopLevelCharts(t *testing.T) {
	t.Setenv(ociCatalogBaseURLEnv, "oci://registry.local/offline")
	t.Setenv(ociCatalogRepositoryNameEnv, "mirror")
	t.Setenv(ociCatalogEnv, `
charts:
  - name: gogs
    version: 0.4.0
`)

	ref, err := FindOCIChartVersion("mirror", "gogs", "")
	require.NoError(t, err)
	require.Equal(t, "mirror", ref.RepositoryName)
	require.Equal(t, "oci://registry.local/offline", ref.RepositoryURL)
	require.Equal(t, "oci://registry.local/offline/gogs:0.4.0", ref.ChartURL)
}

func TestLoadOCIChartCatalogAddsRuntimeRegistryOptions(t *testing.T) {
	t.Setenv(ociCatalogBaseURLEnv, "oci://registry.local/offline")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryInsecureTLSEnv, "true")
	t.Setenv(ociRegistryCAFileEnv, "/etc/kite/registry-ca.crt")
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")
	t.Setenv(ociCatalogEnv, `
charts:
  - name: gogs
    version: 0.4.0
`)

	ref, err := FindOCIChartVersion("offline", "gogs", "")
	require.NoError(t, err)
	require.Equal(t, "oci://registry.local/offline/gogs:0.4.0", ref.ChartURL)
	require.True(t, ref.Registry.PlainHTTP)
	require.True(t, ref.Registry.InsecureSkipTLSVerify)
	require.Equal(t, "/etc/kite/registry-ca.crt", ref.Registry.CAFile)
	require.Equal(t, "admin", ref.Registry.Username)
	require.Equal(t, "secret", ref.Registry.Password)
	require.NotContains(t, ref.ChartURL, "admin")
	require.NotContains(t, ref.ChartURL, "secret")
}

func TestLoadOCIChartCatalogRejectsInvalidRegistryBool(t *testing.T) {
	t.Setenv(ociRegistryPlainHTTPEnv, "maybe")

	_, err := LoadOCIChartCatalog()
	require.Error(t, err)
	require.Contains(t, err.Error(), ociRegistryPlainHTTPEnv)
}

func TestOCIRegistryOptionsForChartURLMatchesCatalog(t *testing.T) {
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0
`)

	options, ok, err := OCIRegistryOptionsForChartURL("oci://registry.local/charts/postgres:12.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, options.PlainHTTP)
	require.Equal(t, "admin", options.Username)
	require.Equal(t, "secret", options.Password)

	options, ok, err = OCIRegistryOptionsForChartURL("oci://registry.local/charts/postgres")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, options.PlainHTTP)
}

func TestLoadOCIChartCatalogRequiresBaseForTopLevelCharts(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
charts:
  - name: gogs
    version: 0.4.0
`)

	_, err := LoadOCIChartCatalog()
	require.Error(t, err)
	require.Contains(t, err.Error(), ociCatalogBaseURLEnv)
}

func TestLoadOCIChartCatalogRejectsSensitiveOCIURLs(t *testing.T) {
	tests := []struct {
		name    string
		catalog string
		want    string
	}{
		{
			name: "repository credentials",
			catalog: `
repositories:
  - name: offline
    url: oci://user:pass@registry.local/charts
    charts:
      - name: postgres
        version: 12.0.0
`,
			want: "credentials",
		},
		{
			name: "chart query",
			catalog: `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        chartUrl: oci://registry.local/charts/postgres?token=secret
        version: 12.0.0
`,
			want: "query",
		},
		{
			name: "version fragment",
			catalog: `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0
            chartUrl: oci://registry.local/charts/postgres#secret
`,
			want: "fragments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ociCatalogEnv, tt.catalog)

			_, err := LoadOCIChartCatalog()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestLoadOCIChartCatalogRejectsMismatchedExplicitReference(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0
            chartUrl: oci://registry.local/charts/postgres:12.1.0
`)

	_, err := LoadOCIChartCatalog()
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match version")
}

func TestLoadOCIChartCatalogRejectsDigestOnlyReference(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0
            chartUrl: oci://registry.local/charts/postgres@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`)

	_, err := LoadOCIChartCatalog()
	require.Error(t, err)
	require.Contains(t, err.Error(), "digest references must include the version tag")
}

func TestLoadOCIChartCatalogAllowsMatchingExplicitReference(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0+build.4
            chartUrl: oci://registry.local/charts/postgres:12.0.0_build.4
`)

	ref, err := FindOCIChartVersion("offline", "postgres", "12.0.0+build.4")
	require.NoError(t, err)
	require.Equal(t, "oci://registry.local/charts/postgres:12.0.0_build.4", ref.ChartURL)
}

func TestOCIChartPackageResolvesConfiguredVersion(t *testing.T) {
	t.Setenv(ociCatalogEnv, `
repositories:
  - name: offline
    url: oci://registry.local/charts
    charts:
      - name: postgres
        versions:
          - version: 12.0.0
          - version: 12.1.0
`)

	pkg, err := ociChartPackage("offline", "postgres", "12.0.0", "oci://registry.local/charts/postgres:12.0.0")
	require.NoError(t, err)
	require.Equal(t, "12.0.0", pkg.Version)
	require.Equal(t, "oci://registry.local/charts/postgres:12.0.0", pkg.URL)

	_, err = ociChartPackage("offline", "postgres", "12.0.0", "oci://registry.local/charts/postgres:12.1.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match")
}

func TestOCIChartVersionURLUsesRegistryTagEncoding(t *testing.T) {
	require.Equal(
		t,
		"oci://registry.local/charts/kite:1.2.3_build.4",
		OCIChartVersionURL("oci://registry.local/charts/kite", "1.2.3+build.4"),
	)
}
