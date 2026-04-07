package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/klog/v2"
)

const (
	desktopDefaultPasswordPrefix = "kite-desktop-auto-"
)

func createDesktopSuperUser(baseUsername, displayName string) (*model.User, error) {
	trimmedBase := strings.TrimSpace(baseUsername)
	if trimmedBase == "" {
		trimmedBase = "admin"
	}

	for i := 0; i < 50; i++ {
		candidate := trimmedBase
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", trimmedBase, i+1)
		}

		autoUser := &model.User{
			Username: candidate,
			Password: fmt.Sprintf("%s%d", desktopDefaultPasswordPrefix, time.Now().UnixNano()),
			Name:     displayName,
			Provider: "password",
			Enabled:  true,
		}

		if err := model.AddSuperUser(autoUser); err == nil {
			return autoUser, nil
		}

		if _, err := model.GetUserByUsername(candidate); err == nil {
			continue
		}
	}

	return nil, fmt.Errorf("failed to create desktop auto-login user with base username %q", trimmedBase)
}

func ensureDesktopAutoLoginUser() (*model.User, bool, error) {
	if !common.DesktopMode {
		return nil, false, nil
	}

	users, err := model.ListEnabledUsersForAutoLogin()
	if err != nil {
		return nil, false, err
	}
	if len(users) > 0 {
		u := users[0]
		return &u, false, nil
	}

	username := strings.TrimSpace(common.DesktopDefaultUsername)
	if username == "" {
		username = "admin"
	}

	displayName := strings.TrimSpace(common.DesktopDefaultName)
	if displayName == "" {
		displayName = "Admin"
	}

	autoUser, err := createDesktopSuperUser(username, displayName)
	if err != nil {
		return nil, false, err
	}

	select {
	case rbac.SyncNow <- struct{}{}:
	default:
	}

	if loginErr := model.LoginUser(autoUser); loginErr != nil {
		return nil, false, loginErr
	}
	return autoUser, true, nil
}

func (h *AuthHandler) tryDesktopAutoLogin(c *gin.Context) (*model.User, bool) {
	if !common.DesktopMode {
		return nil, false
	}

	user, created, err := ensureDesktopAutoLoginUser()
	if err != nil {
		klog.Warningf("Desktop auto-login failed to prepare user: %v", err)
		return nil, false
	}
	if user == nil || !user.Enabled {
		return nil, false
	}

	if loginErr := model.LoginUser(user); loginErr != nil {
		klog.Warningf("Desktop auto-login failed to update last login: %v", loginErr)
	}

	jwtToken, err := h.manager.GenerateJWT(user, "")
	if err != nil {
		klog.Warningf("Desktop auto-login failed to generate JWT: %v", err)
		return nil, false
	}

	setCookieSecure(c, "auth_token", jwtToken, common.CookieExpirationSeconds)

	if created {
		klog.Infof("Desktop auto-login created default user: %s", user.Username)
	} else {
		klog.V(1).Infof("Desktop auto-login reused user: %s", user.Username)
	}

	return user, true
}

func (h *AuthHandler) DesktopAutoLogin(c *gin.Context) {
	user, ok := h.tryDesktopAutoLogin(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "desktop auto-login failed",
		})
		return
	}
	user.Roles = rbac.GetUserRoles(*user)
	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}
