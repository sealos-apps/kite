package helmutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOCIChartVersionsDiscoversRegistryPrefix(t *testing.T) {
	registry := newTestOCIRegistry(t, map[string][]string{
		"kite-helm/redis":      {"1.2.3", "1.3.0", "1.2.3_build.4"},
		"kite-helm/postgres":   {"12.0.0", "not-a-chart"},
		"kite-helm/nested/app": {"1.0.0"},
		"kite-helm2/mysql":     {"8.0.0"},
	})
	ociBase := configureTestOCIRegistry(t, registry, "kite-helm", "mirror")

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Len(t, refs, 4)

	latest, err := LatestOCIChartVersion("mirror", "redis")
	require.NoError(t, err)
	require.Equal(t, "1.3.0", latest.Version.Version)
	require.Equal(t, ociBase+"/redis:1.3.0", latest.ChartURL)
	require.Equal(t, "mirror", latest.RepositoryName)
	require.Equal(t, ociBase, latest.RepositoryURL)

	version, err := FindOCIChartVersion("mirror", "redis", "1.2.3+build.4")
	require.NoError(t, err)
	require.Equal(t, ociBase+"/redis:1.2.3_build.4", version.ChartURL)

	_, err = FindOCIChartVersion("mirror", "mysql", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "chart not found")
}

func TestLoadOCIChartCatalogGroupsDiscoveredVersions(t *testing.T) {
	registry := newTestOCIRegistry(t, map[string][]string{
		"offline/gogs": {"0.3.0", "0.4.0"},
	})
	ociBase := configureTestOCIRegistry(t, registry, "offline", "")

	catalog, err := LoadOCIChartCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.Repositories, 1)
	require.Equal(t, "offline", catalog.Repositories[0].Name)
	require.Equal(t, ociBase, catalog.Repositories[0].URL)
	require.Len(t, catalog.Repositories[0].Charts, 1)
	require.Equal(t, "gogs", catalog.Repositories[0].Charts[0].Name)
	require.Len(t, catalog.Repositories[0].Charts[0].Versions, 2)
}

func TestLoadOCIChartCatalogAddsRuntimeRegistryOptions(t *testing.T) {
	registry := newTestOCIRegistry(t, map[string][]string{
		"offline/gogs": {"0.4.0"},
	})
	configureTestOCIRegistry(t, registry, "offline", "")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryInsecureTLSEnv, "true")
	t.Setenv(ociRegistryCAFileEnv, "/etc/kite/registry-ca.crt")
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")

	ref, err := FindOCIChartVersion("offline", "gogs", "")
	require.NoError(t, err)
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

func TestLoadOCIChartCatalogRejectsInvalidDiscoveryBase(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "credentials",
			base: "oci://user:pass@registry.local/charts",
			want: "credentials",
		},
		{
			name: "query",
			base: "oci://registry.local/charts?token=secret",
			want: "query",
		},
		{
			name: "fragment",
			base: "oci://registry.local/charts#secret",
			want: "fragments",
		},
		{
			name: "tag",
			base: "oci://registry.local/charts/postgres:12.0.0",
			want: "tag or digest",
		},
		{
			name: "host only",
			base: "oci://registry.local",
			want: "repository prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ociRegistryBaseEnv, tt.base)

			_, err := LoadOCIChartCatalog()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestOCIRegistryOptionsForChartURLMatchesDiscoveryPrefix(t *testing.T) {
	t.Setenv(ociRegistryBaseEnv, "oci://registry.local/charts")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")

	options, ok, err := OCIRegistryOptionsForChartURL("oci://registry.local/charts/postgres:12.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, options.PlainHTTP)
	require.Equal(t, "admin", options.Username)
	require.Equal(t, "secret", options.Password)

	_, ok, err = OCIRegistryOptionsForChartURL("oci://registry.local/charts2/postgres:12.0.0")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestOCIChartPackageResolvesDiscoveredVersion(t *testing.T) {
	registry := newTestOCIRegistry(t, map[string][]string{
		"charts/postgres": {"12.0.0", "12.1.0"},
	})
	ociBase := configureTestOCIRegistry(t, registry, "charts", "")

	pkg, err := ociChartPackage("offline", "postgres", "12.0.0", ociBase+"/postgres:12.0.0")
	require.NoError(t, err)
	require.Equal(t, "12.0.0", pkg.Version)
	require.Equal(t, ociBase+"/postgres:12.0.0", pkg.URL)

	_, err = ociChartPackage("offline", "postgres", "12.0.0", ociBase+"/postgres:12.1.0")
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

type testOCIRegistry struct {
	server *httptest.Server
}

func newTestOCIRegistry(t *testing.T, repositories map[string][]string) testOCIRegistry {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, 0, len(repositories))
		for name := range repositories {
			names = append(names, name)
		}
		writeJSON(t, w, map[string]any{"repositories": names})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		pathValue := strings.TrimPrefix(r.URL.Path, "/v2/")
		pathValue = strings.Trim(pathValue, "/")
		if strings.HasSuffix(pathValue, "/tags/list") {
			repositoryName := strings.TrimSuffix(pathValue, "/tags/list")
			tags, ok := repositories[repositoryName]
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(t, w, map[string]any{
				"name": repositoryName,
				"tags": tags,
			})
			return
		}
		parts := strings.Split(pathValue, "/manifests/")
		if len(parts) == 2 {
			tags, ok := repositories[parts[0]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			found := false
			for _, tag := range tags {
				if tag == parts[1] {
					found = true
					break
				}
			}
			if !found {
				http.NotFound(w, r)
				return
			}
			mediaType := helmOCIConfigMediaType
			if parts[1] == "not-a-chart" {
				mediaType = "application/vnd.oci.image.config.v1+json"
			}
			writeJSON(t, w, map[string]any{
				"schemaVersion": 2,
				"config": map[string]any{
					"mediaType": mediaType,
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testOCIRegistry{
		server: server,
	}
}

func configureTestOCIRegistry(t *testing.T, registry testOCIRegistry, prefix, repositoryName string) string {
	t.Helper()
	registryURL := strings.TrimPrefix(registry.server.URL, "http://")
	ociBase := "oci://" + registryURL + "/" + prefix
	t.Setenv(ociRegistryBaseEnv, ociBase)
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	if repositoryName != "" {
		t.Setenv(ociRepositoryNameEnv, repositoryName)
	}
	ClearOCIChartDiscoveryCache()
	return ociBase
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}
