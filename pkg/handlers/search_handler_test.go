package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestFilterSearchResultsHonorsNamespaceRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("user", model.User{
		Username: "alice",
		Roles: []common.Role{
			{
				Name:       "namespace-viewer",
				Clusters:   []string{"cluster-a"},
				Namespaces: []string{"team-a"},
				Resources:  []string{"pods"},
				Verbs:      []string{"get"},
			},
		},
	})
	c.Set("cluster", &cluster.ClientSet{
		Name:            "cluster-a",
		NamespaceScoped: true,
		Namespace:       "team-a",
	})

	results := filterSearchResults(c, []common.SearchResult{
		{Name: "allowed", Namespace: "team-a", ResourceType: "pods"},
		{Name: "leaked", Namespace: "team-b", ResourceType: "pods"},
		{Name: "cluster-node", ResourceType: "nodes"},
	})

	if len(results) != 1 {
		t.Fatalf("expected one visible result, got %#v", results)
	}
	if results[0].Name != "allowed" {
		t.Fatalf("unexpected result: %#v", results[0])
	}
}

func TestSearchCacheKeyIncludesUserAndNamespaceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSearchHandler()

	firstRecorder := httptest.NewRecorder()
	first, _ := gin.CreateTestContext(firstRecorder)
	first.Set("user", model.User{Username: "alice"})
	first.Set("cluster", &cluster.ClientSet{Name: "cluster-a", NamespaceScoped: true, Namespace: "team-a"})

	secondRecorder := httptest.NewRecorder()
	second, _ := gin.CreateTestContext(secondRecorder)
	second.Set("user", model.User{Username: "bob"})
	second.Set("cluster", &cluster.ClientSet{Name: "cluster-a", NamespaceScoped: true, Namespace: "team-b"})

	firstKey := handler.createCacheKey(first, "nginx", 50)
	secondKey := handler.createCacheKey(second, "nginx", 50)
	if firstKey == secondKey {
		t.Fatalf("expected cache key to differ by user and namespace scope")
	}
}
