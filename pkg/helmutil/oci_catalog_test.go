package helmutil

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestOCIChartDiscoverySupportsAnonymousRegistry(t *testing.T) {
	registry, probe := newTestOCIRegistryWithAuth(t, map[string][]string{
		"offline/gogs": {"0.4.0"},
	}, testOCIRegistryAuthAnonymous)
	configureTestOCIRegistry(t, registry, "offline", "")
	t.Setenv(ociRegistryUsernameEnv, testOCIRegistryUsername)
	t.Setenv(ociRegistryPasswordEnv, testOCIRegistryPassword)

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.False(t, probe.sawAuthorization, "anonymous registry requests must not include credentials")
	require.Zero(t, probe.challenges)
}

func TestOCIChartDiscoverySupportsBasicAuthChallenge(t *testing.T) {
	registry, probe := newTestOCIRegistryWithAuth(t, map[string][]string{
		"offline/gogs": {"0.4.0"},
	}, testOCIRegistryAuthBasic)
	configureTestOCIRegistry(t, registry, "offline", "")
	t.Setenv(ociRegistryUsernameEnv, testOCIRegistryUsername)
	t.Setenv(ociRegistryPasswordEnv, testOCIRegistryPassword)

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.GreaterOrEqual(t, probe.challenges, 1, "client must react to the registry's Basic challenge")
	require.GreaterOrEqual(t, probe.authorizedRequests, 1)
}

func TestOCIChartDiscoverySupportsBearerAuthChallenge(t *testing.T) {
	registry, probe := newTestOCIRegistryWithAuth(t, map[string][]string{
		"offline/gogs": {"0.4.0"},
	}, testOCIRegistryAuthBearer)
	configureTestOCIRegistry(t, registry, "offline", "")
	t.Setenv(ociRegistryUsernameEnv, testOCIRegistryUsername)
	t.Setenv(ociRegistryPasswordEnv, testOCIRegistryPassword)

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, 2, probe.challenges, "catalog and repository should each challenge only once")
	require.Equal(t, 2, probe.tokenRequests, "catalog and repository scopes should each exchange one token")
	require.GreaterOrEqual(t, probe.authorizedRequests, 1)
	require.Contains(t, probe.tokenServices, registry.server.Listener.Addr().String())
	require.Contains(t, probe.tokenScopes, "registry:catalog:*")
	require.Contains(t, probe.tokenScopes, "repository:offline/gogs:pull")
}

func TestOCIChartDiscoveryUsesTLSOptionsForBearerTokenRealm(t *testing.T) {
	var tokenRequests atomic.Int32
	tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests.Add(1)
		username, password, ok := r.BasicAuth()
		if !ok || username != testOCIRegistryUsername || password != testOCIRegistryPassword {
			writeRegistryUnauthorized(t, w)
			return
		}
		writeJSON(t, w, map[string]any{"access_token": "fixture-token"})
	}))
	t.Cleanup(tokenServer.Close)

	var registryHost string
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer fixture-token" {
			writeJSON(t, w, map[string]any{"repositories": []string{}})
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(
			`Bearer realm=%q,service=%q,scope=%q`,
			tokenServer.URL,
			registryHost,
			"registry:catalog:*",
		))
		writeRegistryUnauthorized(t, w)
	}))
	t.Cleanup(registryServer.Close)
	parsedRegistryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	registryHost = parsedRegistryURL.Host

	configureTestOCIRegistry(t, testOCIRegistry{server: registryServer}, "offline", "")
	t.Setenv(ociRegistryUsernameEnv, testOCIRegistryUsername)
	t.Setenv(ociRegistryPasswordEnv, testOCIRegistryPassword)
	t.Setenv(ociRegistryInsecureTLSEnv, "true")

	refs, err := ListOCIChartVersions()
	require.NoError(t, err)
	require.Empty(t, refs)
	require.Equal(t, int32(1), tokenRequests.Load())
}

func TestOCIChartDiscoveryRejectsInvalidRegistryCredentials(t *testing.T) {
	for _, authMode := range []testOCIRegistryAuthMode{
		testOCIRegistryAuthBasic,
		testOCIRegistryAuthBearer,
	} {
		t.Run(string(authMode), func(t *testing.T) {
			registry, probe := newTestOCIRegistryWithAuth(t, map[string][]string{
				"offline/gogs": {"0.4.0"},
			}, authMode)
			configureTestOCIRegistry(t, registry, "offline", "")
			t.Setenv(ociRegistryUsernameEnv, testOCIRegistryUsername)
			t.Setenv(ociRegistryPasswordEnv, "incorrect-fixture-password")

			_, err := ListOCIChartVersions()
			require.Error(t, err)
			require.Contains(t, err.Error(), "401")
			require.NotContains(t, err.Error(), "incorrect-fixture-password")
			require.GreaterOrEqual(t, probe.challenges, 1)
			require.Zero(t, probe.authorizedRequests)
		})
	}
}

func TestLoadOCIChartCatalogAddsRuntimeRegistryOptions(t *testing.T) {
	registry := newTestOCIRegistry(t, map[string][]string{
		"offline/gogs": {"0.4.0"},
	})
	caServer := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(caServer.Close)
	caFile := filepath.Join(t.TempDir(), "registry-ca.crt")
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caServer.Certificate().Raw})
	require.NoError(t, os.WriteFile(caFile, caData, 0o600))

	configureTestOCIRegistry(t, registry, "offline", "")
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryInsecureTLSEnv, "true")
	t.Setenv(ociRegistryCAFileEnv, caFile)
	t.Setenv(ociRegistryUsernameEnv, "admin")
	t.Setenv(ociRegistryPasswordEnv, "secret")

	ref, err := FindOCIChartVersion("offline", "gogs", "")
	require.NoError(t, err)
	require.True(t, ref.Registry.PlainHTTP)
	require.True(t, ref.Registry.InsecureSkipTLSVerify)
	require.Equal(t, caFile, ref.Registry.CAFile)
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

type testOCIRegistryAuthMode string

const (
	testOCIRegistryAuthAnonymous testOCIRegistryAuthMode = "anonymous"
	testOCIRegistryAuthBasic     testOCIRegistryAuthMode = "basic"
	testOCIRegistryAuthBearer    testOCIRegistryAuthMode = "bearer"

	testOCIRegistryUsername = "fixture-user"
	testOCIRegistryPassword = "fixture-password"
)

type testOCIRegistryAuthProbe struct {
	challenges         int
	tokenRequests      int
	authorizedRequests int
	sawAuthorization   bool
	tokenServices      []string
	tokenScopes        []string
}

func newTestOCIRegistry(t *testing.T, repositories map[string][]string) testOCIRegistry {
	registry, _ := newTestOCIRegistryWithAuth(t, repositories, testOCIRegistryAuthAnonymous)
	return registry
}

func newTestOCIRegistryWithAuth(t *testing.T, repositories map[string][]string, authMode testOCIRegistryAuthMode) (testOCIRegistry, *testOCIRegistryAuthProbe) {
	t.Helper()
	probe := &testOCIRegistryAuthProbe{}
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		probe.tokenRequests++
		probe.tokenServices = append(probe.tokenServices, r.URL.Query().Get("service"))
		probe.tokenScopes = append(probe.tokenScopes, r.URL.Query()["scope"]...)
		username, password, ok := r.BasicAuth()
		if !ok || username != testOCIRegistryUsername || password != testOCIRegistryPassword {
			writeRegistryUnauthorized(t, w)
			return
		}
		writeJSON(t, w, map[string]any{"access_token": testOCIRegistryToken(r.URL.Query().Get("scope"))})
	})
	mux.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeTestOCIRegistryRequest(t, w, r, server, authMode, probe) {
			return
		}
		names := make([]string, 0, len(repositories))
		for name := range repositories {
			names = append(names, name)
		}
		writeJSON(t, w, map[string]any{"repositories": names})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeTestOCIRegistryRequest(t, w, r, server, authMode, probe) {
			return
		}
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
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testOCIRegistry{
		server: server,
	}, probe
}

func authorizeTestOCIRegistryRequest(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	server *httptest.Server,
	authMode testOCIRegistryAuthMode,
	probe *testOCIRegistryAuthProbe,
) bool {
	t.Helper()
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		probe.sawAuthorization = true
	}

	switch authMode {
	case testOCIRegistryAuthAnonymous:
		if authorization != "" {
			http.Error(w, "anonymous registry received an authorization header", http.StatusBadRequest)
			return false
		}
		probe.authorizedRequests++
		return true
	case testOCIRegistryAuthBasic:
		username, password, ok := r.BasicAuth()
		if ok && username == testOCIRegistryUsername && password == testOCIRegistryPassword {
			probe.authorizedRequests++
			return true
		}
		probe.challenges++
		w.Header().Set("WWW-Authenticate", `Basic realm="kite-test-registry"`)
		writeRegistryUnauthorized(t, w)
		return false
	case testOCIRegistryAuthBearer:
		if authorization == "Bearer "+testOCIRegistryToken(testOCIRegistryScope(r.URL.Path)) {
			probe.authorizedRequests++
			return true
		}
		probe.challenges++
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(
			`Bearer realm=%q,service=%q,scope=%q`,
			server.URL+"/token",
			r.Host,
			testOCIRegistryScope(r.URL.Path),
		))
		writeRegistryUnauthorized(t, w)
		return false
	default:
		t.Fatalf("unsupported test OCI registry auth mode %q", authMode)
		return false
	}
}

func testOCIRegistryScope(requestPath string) string {
	if requestPath == "/v2/_catalog" {
		return "registry:catalog:*"
	}
	repository := strings.TrimPrefix(requestPath, "/v2/")
	if index := strings.Index(repository, "/tags/list"); index >= 0 {
		repository = repository[:index]
	} else if index := strings.Index(repository, "/manifests/"); index >= 0 {
		repository = repository[:index]
	}
	return "repository:" + repository + ":pull"
}

func testOCIRegistryToken(scope string) string {
	return "fixture-access-token-for-" + scope
}

func writeRegistryUnauthorized(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.WriteHeader(http.StatusUnauthorized)
	writeJSON(t, w, map[string]any{
		"errors": []map[string]any{{
			"code":    "UNAUTHORIZED",
			"message": "authentication required",
		}},
	})
}

func configureTestOCIRegistry(t *testing.T, registry testOCIRegistry, prefix, repositoryName string) string {
	t.Helper()
	registryURL := strings.TrimPrefix(registry.server.URL, "http://")
	ociBase := "oci://" + registryURL + "/" + prefix
	t.Setenv(ociRegistryBaseEnv, ociBase)
	t.Setenv(ociRegistryPlainHTTPEnv, "true")
	t.Setenv(ociRegistryUsernameEnv, "")
	t.Setenv(ociRegistryPasswordEnv, "")
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
