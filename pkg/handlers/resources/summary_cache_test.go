package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/client-go/rest"
)

func TestSummaryListCacheCopiesAndExpires(t *testing.T) {
	cache := newSummaryListCache(10*time.Millisecond, 2)
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	cache.set("pods", []byte(`{"items":[]}`), now)
	body, state, ok := cache.get("pods", now.Add(5*time.Millisecond))
	if !ok {
		t.Fatalf("expected cache hit before TTL")
	}
	if state != summaryListCacheFresh {
		t.Fatalf("cache state = %q, want fresh", state)
	}
	body[0] = '['

	body, state, ok = cache.get("pods", now.Add(5*time.Millisecond))
	if !ok {
		t.Fatalf("expected cache hit before TTL after caller mutation")
	}
	if state != summaryListCacheFresh {
		t.Fatalf("cache state after caller mutation = %q, want fresh", state)
	}
	if got := string(body); got != `{"items":[]}` {
		t.Fatalf("cached body was mutated: %s", got)
	}

	_, state, ok = cache.get("pods", now.Add(11*time.Millisecond))
	if !ok {
		t.Fatalf("expected stale cache hit after fresh TTL")
	}
	if state != summaryListCacheStale {
		t.Fatalf("cache state after fresh TTL = %q, want stale", state)
	}

	if _, _, ok := cache.get("pods", now.Add(summaryListCacheStaleTTL+time.Second)); ok {
		t.Fatalf("expected cache miss after stale TTL")
	}
}

func TestSummaryListCacheEvictsAtCapacity(t *testing.T) {
	cache := newSummaryListCache(time.Minute, 1)
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	cache.set("pods", []byte(`{"items":[1]}`), now)
	cache.set("services", []byte(`{"items":[2]}`), now)

	if _, _, ok := cache.get("services", now); !ok {
		t.Fatalf("expected newest entry to remain")
	}
	if len(cache.entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(cache.entries))
	}
}

func TestSummaryListCacheKeySeparatesUsersAndSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	base := newSummaryCacheTestContext("/api/v1/pods/default?summary=true&reduce=true&limit=100", "cluster-a", "alice", "default")
	sameClusterDifferentUser := newSummaryCacheTestContext("/api/v1/pods/default?summary=true&reduce=true&limit=100", "cluster-a", "bob", "default")
	sameUserDifferentSelector := newSummaryCacheTestContext("/api/v1/pods/default?summary=true&reduce=true&limit=100&labelSelector=app%3Dapi", "cluster-a", "alice", "default")

	baseKey := summaryListCacheKey(base, common.Pods)
	if baseKey == summaryListCacheKey(sameClusterDifferentUser, common.Pods) {
		t.Fatalf("cache key must include user identity")
	}
	if baseKey == summaryListCacheKey(sameUserDifferentSelector, common.Pods) {
		t.Fatalf("cache key must include selectors")
	}
}

func TestWriteCachedSummaryListUsesCachedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clearSummaryListCache()
	defer clearSummaryListCache()

	calls := 0
	load := func(*gin.Context) (*summaryListResponse, error) {
		calls++
		return &summaryListResponse{
			APIVersion: "v1",
			Kind:       "PodList",
			Items: []map[string]any{{
				"metadata": map[string]any{"name": "api-0"},
			}},
		}, nil
	}

	first, firstRecorder := newSummaryCacheTestContextWithRecorder("/api/v1/pods/default?summary=true&reduce=true&limit=100", "cluster-a", "alice", "default")
	handled, err := writeCachedSummaryList(first, common.Pods, load)
	if err != nil || !handled {
		t.Fatalf("first writeCachedSummaryList handled=%t err=%v", handled, err)
	}
	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
	if got := firstRecorder.Header().Get(summaryListCacheHeader); got != "miss" {
		t.Fatalf("first cache header = %q, want miss", got)
	}

	second, secondRecorder := newSummaryCacheTestContextWithRecorder("/api/v1/pods/default?summary=true&reduce=true&limit=100", "cluster-a", "alice", "default")
	handled, err = writeCachedSummaryList(second, common.Pods, load)
	if err != nil || !handled {
		t.Fatalf("second writeCachedSummaryList handled=%t err=%v", handled, err)
	}
	if calls != 1 {
		t.Fatalf("loader calls after hit = %d, want 1", calls)
	}
	if got := secondRecorder.Header().Get(summaryListCacheHeader); got != "hit" {
		t.Fatalf("second cache header = %q, want hit", got)
	}
	if body := secondRecorder.Body.String(); !strings.Contains(body, `"api-0"`) {
		t.Fatalf("cached response body = %s", body)
	}
}

func newSummaryCacheTestContext(rawURL, clusterName, username, namespace string) *gin.Context {
	ctx, _ := newSummaryCacheTestContextWithRecorder(rawURL, clusterName, username, namespace)
	return ctx
}

func newSummaryCacheTestContextWithRecorder(rawURL, clusterName, username, namespace string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	ctx.Params = gin.Params{{Key: "namespace", Value: namespace}}
	ctx.Set("cluster", &cluster.ClientSet{
		Name: clusterName,
		K8sClient: &kube.K8sClient{
			Configuration: &rest.Config{Host: "https://cluster.example"},
		},
	})
	ctx.Set("user", model.User{Username: username})
	return ctx, recorder
}
