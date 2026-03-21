package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/zxh326/kite/pkg/common"
)

func TestResolveCookieSameSite_SealosDefault(t *testing.T) {
	originalSealosEnabled := common.SealosAuthEnabled
	originalSameSite := common.AuthCookieSameSite
	originalSameSiteExplicit := common.AuthCookieSameSiteExplicit
	t.Cleanup(func() {
		common.SealosAuthEnabled = originalSealosEnabled
		common.AuthCookieSameSite = originalSameSite
		common.AuthCookieSameSiteExplicit = originalSameSiteExplicit
	})

	common.SealosAuthEnabled = true
	common.AuthCookieSameSite = "lax"
	common.AuthCookieSameSiteExplicit = false

	assert.Equal(t, http.SameSiteNoneMode, resolveCookieSameSite())
}

func TestResolveCookieSameSite_RespectExplicitConfig(t *testing.T) {
	originalSealosEnabled := common.SealosAuthEnabled
	originalSameSite := common.AuthCookieSameSite
	originalSameSiteExplicit := common.AuthCookieSameSiteExplicit
	t.Cleanup(func() {
		common.SealosAuthEnabled = originalSealosEnabled
		common.AuthCookieSameSite = originalSameSite
		common.AuthCookieSameSiteExplicit = originalSameSiteExplicit
	})

	common.SealosAuthEnabled = true
	common.AuthCookieSameSite = "lax"
	common.AuthCookieSameSiteExplicit = true

	assert.Equal(t, http.SameSiteLaxMode, resolveCookieSameSite())
}

func TestResolveCookieSecure_ForceSecureForSameSiteNone(t *testing.T) {
	originalSealosEnabled := common.SealosAuthEnabled
	originalSameSite := common.AuthCookieSameSite
	originalSameSiteExplicit := common.AuthCookieSameSiteExplicit
	originalSecure := common.AuthCookieSecure
	t.Cleanup(func() {
		common.SealosAuthEnabled = originalSealosEnabled
		common.AuthCookieSameSite = originalSameSite
		common.AuthCookieSameSiteExplicit = originalSameSiteExplicit
		common.AuthCookieSecure = originalSecure
	})

	common.SealosAuthEnabled = true
	common.AuthCookieSameSite = "none"
	common.AuthCookieSameSiteExplicit = true
	common.AuthCookieSecure = "false"

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "http://example.local/api/auth/user", nil)
	c.Request = req

	assert.True(t, resolveCookieSecure(c))
}
