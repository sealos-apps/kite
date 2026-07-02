package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
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
		{http.MethodPost, "/api/v1/admin/charts/offline-bundles/import-jobs"},
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

func TestOfflineBundleImportJobLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan string, 1)
	release := make(chan struct{})
	handler := NewHelmChartHandler()
	handler.offlineBundleJobs = newOfflineBundleImportJobStore(
		func(_ context.Context, bundlePath string, _ helmutil.BundleImportOptions) (helmutil.BundleImportResult, error) {
			data, err := os.ReadFile(bundlePath)
			if err != nil {
				return helmutil.BundleImportResult{}, err
			}
			if string(data) != "bundle data" {
				return helmutil.BundleImportResult{}, errors.New("unexpected bundle data")
			}
			started <- bundlePath
			<-release
			return helmutil.BundleImportResult{
				Apps: []helmutil.BundleImportAppResult{{
					Name:    "nginx",
					Version: "25.0.12",
				}},
			}, nil
		},
	)
	router := gin.New()
	handler.RegisterAdminRoutes(router.Group("/api/v1/admin"))

	recorder := httptest.NewRecorder()
	req := newMultipartFileRequest(t, "/api/v1/admin/charts/offline-bundles/import-jobs", "bundle.kiteapp.tar.gz", []byte("bundle data"))
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	var created offlineBundleImportJob
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	require.Contains(t, []offlineBundleImportJobStatus{offlineBundleImportQueued, offlineBundleImportRunning}, created.Status)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("import job did not start")
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/offline-bundles/import-jobs/"+created.ID, nil)
	statusRecorder := httptest.NewRecorder()
	router.ServeHTTP(statusRecorder, statusReq)
	require.Equal(t, http.StatusOK, statusRecorder.Code)
	var running offlineBundleImportJob
	require.NoError(t, json.Unmarshal(statusRecorder.Body.Bytes(), &running))
	require.Equal(t, offlineBundleImportRunning, running.Status)

	close(release)
	require.Eventually(t, func() bool {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/offline-bundles/import-jobs/"+created.ID, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			return false
		}
		var completed offlineBundleImportJob
		if err := json.Unmarshal(recorder.Body.Bytes(), &completed); err != nil {
			return false
		}
		return completed.Status == offlineBundleImportSucceeded &&
			completed.Result != nil &&
			len(completed.Result.Apps) == 1 &&
			completed.CompletedAt != nil
	}, 2*time.Second, 20*time.Millisecond)
}

func TestOfflineBundleImportJobFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHelmChartHandler()
	handler.offlineBundleJobs = newOfflineBundleImportJobStore(
		func(context.Context, string, helmutil.BundleImportOptions) (helmutil.BundleImportResult, error) {
			return helmutil.BundleImportResult{}, errors.New("registry unavailable")
		},
	)
	router := gin.New()
	handler.RegisterAdminRoutes(router.Group("/api/v1/admin"))

	recorder := httptest.NewRecorder()
	req := newMultipartFileRequest(t, "/api/v1/admin/charts/offline-bundles/import-jobs", "bundle.kiteapp.tar.gz", []byte("bundle data"))
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	var created offlineBundleImportJob
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &created))
	require.Eventually(t, func() bool {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/offline-bundles/import-jobs/"+created.ID, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			return false
		}
		var completed offlineBundleImportJob
		if err := json.Unmarshal(recorder.Body.Bytes(), &completed); err != nil {
			return false
		}
		return completed.Status == offlineBundleImportFailed &&
			completed.Error == "registry unavailable" &&
			completed.CompletedAt != nil
	}, 2*time.Second, 20*time.Millisecond)
}

func TestOfflineBundleImportJobNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHelmChartHandler().RegisterAdminRoutes(router.Group("/api/v1/admin"))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/charts/offline-bundles/import-jobs/missing", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "not found")
}

func TestOfflineBundleImportJobStoreDoesNotTrimRunningJobs(t *testing.T) {
	store := newOfflineBundleImportJobStore(
		func(context.Context, string, helmutil.BundleImportOptions) (helmutil.BundleImportResult, error) {
			return helmutil.BundleImportResult{}, nil
		},
	)
	now := time.Now().UTC()
	store.jobs["running"] = &offlineBundleImportJob{
		ID:        "running",
		Status:    offlineBundleImportRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.order = append(store.order, "running")
	for i := 0; i < maxOfflineBundleImportJobs; i++ {
		id := fmt.Sprintf("completed-%03d", i)
		store.jobs[id] = &offlineBundleImportJob{
			ID:        id,
			Status:    offlineBundleImportSucceeded,
			CreatedAt: now,
			UpdatedAt: now,
		}
		store.order = append(store.order, id)
	}

	store.trimLocked()

	_, ok := store.jobs["running"]
	require.True(t, ok)
	require.Len(t, store.order, maxOfflineBundleImportJobs)
	require.Len(t, store.jobs, maxOfflineBundleImportJobs)
}

func newMultipartFileRequest(t *testing.T, target, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = file.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
