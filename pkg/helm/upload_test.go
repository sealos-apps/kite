package helm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
)

func TestGetRepositoryUploadConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("KITE_HELM_OCI_REGISTRY_BASE", "oci://registry.local/kite-helm")
	t.Setenv("KITE_IMAGE_UPLOAD_REGISTRY", "hub.local")
	t.Setenv("KITE_IMAGE_UPLOAD_REPOSITORY_PREFIX", "offline")

	router := gin.New()
	NewHelmChartHandler().RegisterAdminRoutes(router.Group("/api/v1/admin"))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/uploads/config", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["chart"]["configured"])
	require.Equal(t, "oci://registry.local/kite-helm", body["chart"]["registryBase"])
	require.Equal(t, true, body["image"]["configured"])
	require.Equal(t, "hub.local", body["image"]["registry"])
	require.Equal(t, "offline", body["image"]["repositoryPrefix"])
}

func TestUploadRoutesDoNotConflictWithChartWildcards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("KITE_HELM_OCI_REGISTRY_BASE", "oci://registry.local/kite-helm")
	router := gin.New()
	NewHelmChartHandler().RegisterAdminRoutes(router.Group("/api/v1/admin"))

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/charts/uploads/config"},
		{http.MethodGet, "/api/v1/admin/charts/offline-bundles/config"},
		{http.MethodPost, "/api/v1/admin/charts/offline-bundles/import"},
		{http.MethodPost, "/api/v1/admin/charts/offline-bundles/export"},
	} {
		t.Run(route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(route.method, route.path, nil)
			router.ServeHTTP(recorder, req)
			require.NotEqual(t, http.StatusNotFound, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "repository not found")
		})
	}
}

func TestGetOfflineBundleConfigUsesOfflineImageRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("KITE_HELM_OCI_REGISTRY_BASE", "oci://registry.local/kite-helm")
	t.Setenv("KITE_IMAGE_UPLOAD_REGISTRY", "manual-upload.local")
	originalRegistry := common.HelmOfflineImagesRegistry
	common.HelmOfflineImagesRegistry = "offline-images.local"
	t.Cleanup(func() {
		common.HelmOfflineImagesRegistry = originalRegistry
	})

	router := gin.New()
	NewHelmChartHandler().RegisterAdminRoutes(router.Group("/api/v1/admin"))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/offline-bundles/config", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["chart"]["configured"])
	require.Equal(t, true, body["image"]["configured"])
	require.Equal(t, "offline-images.local", body["image"]["registry"])
	require.Empty(t, body["image"]["repositoryPrefix"])
}
