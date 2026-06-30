package helm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/helmutil"
)

func TestListOCIChartsReturnsLatestDiscoveredVersion(t *testing.T) {
	registry := newHelmTestOCIRegistry(t, map[string][]string{
		"kite-helm/redis":    {"1.9.0", "1.10.0"},
		"kite-helm/postgres": {"12.0.0"},
		"kite-helm2/mysql":   {"8.0.0"},
	})
	configureHelmTestOCIRegistry(t, registry, "kite-helm", "mirror")

	handler := NewHelmChartHandler()
	items, err := handler.listOCICharts("", "")
	require.NoError(t, err)
	require.Len(t, items, 2)

	require.Equal(t, "postgres", items[0].Name)
	require.Equal(t, "12.0.0", items[0].Version)
	require.Equal(t, helmutil.ChartSourceOCI, items[0].Source)
	require.Equal(t, "mirror", items[0].RepositoryName)

	require.Equal(t, "redis", items[1].Name)
	require.Equal(t, "1.10.0", items[1].Version)
	require.Equal(t, registry.ociBase+"/redis:1.10.0", items[1].ChartURL)
}

func TestListOCIChartsFiltersRepositoryAndQuery(t *testing.T) {
	registry := newHelmTestOCIRegistry(t, map[string][]string{
		"kite-helm/redis":    {"1.0.0"},
		"kite-helm/postgres": {"12.0.0"},
	})
	configureHelmTestOCIRegistry(t, registry, "kite-helm", "mirror")

	handler := NewHelmChartHandler()
	items, err := handler.listOCICharts("other", "")
	require.NoError(t, err)
	require.Empty(t, items)

	items, err = handler.listOCICharts("mirror", "redis")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "redis", items[0].Name)
}

type helmTestOCIRegistry struct {
	server  *httptest.Server
	ociBase string
}

func newHelmTestOCIRegistry(t *testing.T, repositories map[string][]string) helmTestOCIRegistry {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, 0, len(repositories))
		for name := range repositories {
			names = append(names, name)
		}
		writeHelmJSON(t, w, map[string]any{"repositories": names})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		pathValue := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v2/"), "/")
		if strings.HasSuffix(pathValue, "/tags/list") {
			repositoryName := strings.TrimSuffix(pathValue, "/tags/list")
			tags, ok := repositories[repositoryName]
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeHelmJSON(t, w, map[string]any{"name": repositoryName, "tags": tags})
			return
		}
		parts := strings.Split(pathValue, "/manifests/")
		if len(parts) == 2 {
			writeHelmJSON(t, w, map[string]any{
				"schemaVersion": 2,
				"config": map[string]any{
					"mediaType": "application/vnd.cncf.helm.config.v1+json",
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	registryURL := strings.TrimPrefix(server.URL, "http://")
	return helmTestOCIRegistry{
		server:  server,
		ociBase: "oci://" + registryURL + "/kite-helm",
	}
}

func configureHelmTestOCIRegistry(t *testing.T, registry helmTestOCIRegistry, prefix, repositoryName string) {
	t.Helper()
	registryURL := strings.TrimPrefix(registry.server.URL, "http://")
	t.Setenv("KITE_HELM_OCI_REGISTRY_BASE", "oci://"+registryURL+"/"+prefix)
	t.Setenv("KITE_HELM_OCI_REGISTRY_PLAIN_HTTP", "true")
	t.Setenv("KITE_HELM_OCI_REPOSITORY_NAME", repositoryName)
	helmutil.ClearOCIChartDiscoveryCache()
}

func writeHelmJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}
