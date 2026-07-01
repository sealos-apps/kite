package helm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/uploads/config", nil)
	router.ServeHTTP(recorder, req)
	require.NotEqual(t, http.StatusNotFound, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "repository not found")
}
