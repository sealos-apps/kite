package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
)

func TestUrl2NamespaceResource(t *testing.T) {
	testCases := []struct {
		name          string
		url           string
		wantNamespace string
		wantResource  string
	}{
		{
			name:          "valid URL with namespace and resource",
			url:           "/api/v1/pods/default/pods",
			wantNamespace: "default",
			wantResource:  "pods",
		},
		{
			name:          "valid URL with all namespace and specific resource",
			url:           "/api/v1/pvs/_all/some-pv",
			wantNamespace: "_all",
			wantResource:  "pvs",
		},
		{
			name:          "valid URL with namespace only",
			url:           "/api/v1/pods/default",
			wantNamespace: "default",
			wantResource:  "pods",
		},
		{
			name:          "invalid URL - too short (3 parts)",
			url:           "/api/v1",
			wantNamespace: "",
			wantResource:  "",
		},
		{
			name:          "invalid URL - missing namespace",
			url:           "/api/v1/pods",
			wantNamespace: "_all",
			wantResource:  "pods",
		},
		{
			name:          "URL with additional parts",
			url:           "/api/v1/pods/default/some-pods",
			wantNamespace: "default",
			wantResource:  "pods",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotNamespace, gotResource := url2namespaceresource(tc.url)
			if gotNamespace != tc.wantNamespace || gotResource != tc.wantResource {
				t.Errorf("url2namespaceresource(%q) = (%q, %q), want (%q, %q)",
					tc.url, gotNamespace, gotResource, tc.wantNamespace, tc.wantResource)
			}
		})
	}
}

func TestRBACMiddlewareNormalizesAllNamespaceForNamespaceScopedCluster(t *testing.T) {
	router := newNamespaceScopedRBACRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/configmaps/_all/app-config", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /_all returned status %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestRBACMiddlewareRejectsOutsideNamespaceScopedCluster(t *testing.T) {
	router := newNamespaceScopedRBACRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/configmaps/team-b/app-config", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("GET outside namespace returned status %d, want %d", w.Code, http.StatusForbidden)
	}
	if got := w.Body.String(); got == "" || !strings.Contains(got, "outside the current workspace scope team-a") {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func newNamespaceScopedRBACRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousConfig := rbac.RBACConfig
	t.Cleanup(func() {
		rbac.RBACConfig = previousConfig
	})
	rbac.RBACConfig = &common.RolesConfig{
		Roles: []common.Role{
			{
				Name:       "scoped",
				Clusters:   []string{"test-cluster"},
				Namespaces: []string{"team-a"},
				Resources:  []string{"configmaps"},
				Verbs:      []string{"get"},
			},
		},
		RoleMapping: []common.RoleMapping{
			{
				Name:  "scoped",
				Users: []string{"alice"},
			},
		},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user", model.User{Username: "alice"})
		c.Set("cluster", &cluster.ClientSet{
			Name:            "test-cluster",
			NamespaceScoped: true,
			Namespace:       "team-a",
		})
	})
	router.Use(RBACMiddleware())
	router.GET("/api/v1/configmaps/_all/app-config", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/api/v1/configmaps/team-b/app-config", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}
