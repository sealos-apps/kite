package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

const (
	summaryListCacheTTL        = 10 * time.Second
	summaryListCacheStaleTTL   = 60 * time.Second
	summaryListRefreshTimeout  = 15 * time.Second
	summaryListCacheMaxEntries = 512
	summaryListCacheHeader     = "X-Kite-Summary-Cache"
)

var summaryListCache = newSummaryListCache(summaryListCacheTTL, summaryListCacheMaxEntries)

type summaryListCacheEntry struct {
	body       []byte
	expiresAt  time.Time
	staleUntil time.Time
	refreshing bool
}

type summaryListCacheStore struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	entries    map[string]summaryListCacheEntry
}

func newSummaryListCache(ttl time.Duration, maxEntries int) *summaryListCacheStore {
	return &summaryListCacheStore{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    map[string]summaryListCacheEntry{},
	}
}

type summaryListCacheState string

const (
	summaryListCacheFresh summaryListCacheState = "hit"
	summaryListCacheStale summaryListCacheState = "stale"
)

func (s *summaryListCacheStore) get(key string, now time.Time) ([]byte, summaryListCacheState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok {
		return nil, "", false
	}
	if !entry.staleUntil.After(now) {
		delete(s.entries, key)
		return nil, "", false
	}
	if entry.expiresAt.After(now) {
		return cloneBytes(entry.body), summaryListCacheFresh, true
	}
	return cloneBytes(entry.body), summaryListCacheStale, true
}

func (s *summaryListCacheStore) set(key string, body []byte, now time.Time) {
	if s.maxEntries <= 0 || s.ttl <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[key]; !exists && len(s.entries) >= s.maxEntries {
		for evictKey := range s.entries {
			delete(s.entries, evictKey)
			break
		}
	}

	s.entries[key] = summaryListCacheEntry{
		body:       cloneBytes(body),
		expiresAt:  now.Add(s.ttl),
		staleUntil: now.Add(summaryListCacheStaleTTL),
	}
}

func (s *summaryListCacheStore) markRefreshing(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok || !entry.staleUntil.After(now) || entry.refreshing {
		return false
	}
	entry.refreshing = true
	s.entries[key] = entry
	return true
}

func (s *summaryListCacheStore) finishRefresh(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok {
		return
	}
	entry.refreshing = false
	s.entries[key] = entry
}

func (s *summaryListCacheStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = map[string]summaryListCacheEntry{}
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func clearSummaryListCache() {
	summaryListCache.clear()
}

func writeCachedSummaryList(c *gin.Context, resource common.ResourceType, load func(*gin.Context) (*summaryListResponse, error)) (bool, error) {
	if !wantsSummaryList(c) {
		return false, nil
	}

	key := summaryListCacheKey(c, resource)
	now := time.Now()
	if body, state, ok := summaryListCache.get(key, now); ok {
		c.Header(summaryListCacheHeader, string(state))
		if state == summaryListCacheStale {
			refreshCachedSummaryList(c, key, load)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return true, nil
	}

	object, err := load(c)
	if err != nil {
		return false, err
	}
	body, err := json.Marshal(object)
	if err != nil {
		return false, err
	}
	summaryListCache.set(key, body, now)
	c.Header(summaryListCacheHeader, "miss")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return true, nil
}

func refreshCachedSummaryList(c *gin.Context, key string, load func(*gin.Context) (*summaryListResponse, error)) {
	if !summaryListCache.markRefreshing(key, time.Now()) {
		return
	}

	bg := c.Copy()
	ctx, cancel := context.WithTimeout(context.Background(), summaryListRefreshTimeout)
	bg.Request = c.Request.Clone(ctx)

	go func() {
		defer cancel()
		defer summaryListCache.finishRefresh(key)

		object, err := load(bg)
		if err != nil {
			return
		}
		body, err := json.Marshal(object)
		if err != nil {
			return
		}
		summaryListCache.set(key, body, time.Now())
	}()
}

func summaryListCacheKey(c *gin.Context, resource common.ResourceType) string {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	host := ""
	if cs.K8sClient != nil && cs.K8sClient.Configuration != nil {
		host = cs.K8sClient.Configuration.Host
	}

	return strings.Join([]string{
		cs.Name,
		host,
		user.Key(),
		string(resource),
		c.Param("namespace"),
		c.Query("limit"),
		c.Query("continue"),
		c.Query("labelSelector"),
		c.Query("fieldSelector"),
	}, "\x00")
}
