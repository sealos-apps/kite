package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenericResourceGetUsesNamespaceScopeForAllNamespaces(t *testing.T) {
	router := newNamespaceScopedResourceRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/configmaps/_all/app-config", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response corev1.ConfigMap
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "team-a", response.Namespace)
	require.Equal(t, "app-config", response.Name)
}

func TestGenericResourceGetRejectsOutsideNamespaceScope(t *testing.T) {
	router := newNamespaceScopedResourceRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/configmaps/team-b/app-config", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "outside the current workspace scope team-a")
}

func newNamespaceScopedResourceRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "team-a",
			Name:      "app-config",
		},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("cluster", &cluster.ClientSet{
			Name:            "test-cluster",
			NamespaceScoped: true,
			Namespace:       "team-a",
			K8sClient: &kube.K8sClient{
				Client: fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(configMap).
					Build(),
			},
		})
	})
	RegisterRoutes(router.Group("/api/v1"))
	return router
}
