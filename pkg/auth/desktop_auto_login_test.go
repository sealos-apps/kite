package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
)

var (
	desktopAutoLoginTestInitOnce sync.Once
)

func setupDesktopAutoLoginTestDB(t *testing.T) {
	t.Helper()

	originalDBType := common.DBType
	originalDBDSN := common.DBDSN
	originalDesktopMode := common.DesktopMode
	originalDesktopDefaultUsername := common.DesktopDefaultUsername
	originalDesktopDefaultName := common.DesktopDefaultName
	originalAnonymousEnabled := common.AnonymousUserEnabled

	desktopAutoLoginTestInitOnce.Do(func() {
		dbDir, err := os.MkdirTemp("", "kite-desktop-auto-login-*")
		require.NoError(t, err)

		common.DBType = "sqlite"
		common.DBDSN = filepath.Join(dbDir, "desktop-auto-login-test.db")
		common.DesktopMode = true
		common.DesktopDefaultUsername = "admin"
		common.DesktopDefaultName = "Admin"
		common.AnonymousUserEnabled = false

		model.InitDB()
		rbac.InitRBAC()
		require.NoError(t, rbac.ForceSyncRolesConfig())
	})

	common.DesktopMode = true
	common.DesktopDefaultUsername = "admin"
	common.DesktopDefaultName = "Admin"
	common.AnonymousUserEnabled = false

	require.NoError(t, model.DB.Exec("DELETE FROM role_assignments WHERE subject_type = ?", model.SubjectTypeUser).Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)

	t.Cleanup(func() {
		common.DBType = originalDBType
		common.DBDSN = originalDBDSN
		common.DesktopMode = originalDesktopMode
		common.DesktopDefaultUsername = originalDesktopDefaultUsername
		common.DesktopDefaultName = originalDesktopDefaultName
		common.AnonymousUserEnabled = originalAnonymousEnabled
	})
}

func TestEnsureDesktopAutoLoginUser_CreatesDefaultWhenEmpty(t *testing.T) {
	setupDesktopAutoLoginTestDB(t)

	user, created, err := ensureDesktopAutoLoginUser()
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, user)
	require.Equal(t, "admin", user.Username)
	require.Equal(t, "Admin", user.Name)
	require.True(t, user.Enabled)

	count, err := model.CountUsers()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	// Ensure it remains idempotent and reuses existing user.
	reused, createdAgain, err := ensureDesktopAutoLoginUser()
	require.NoError(t, err)
	require.False(t, createdAgain)
	require.NotNil(t, reused)
	require.Equal(t, user.ID, reused.ID)
}

func TestEnsureDesktopAutoLoginUser_PrefersRecentlyLoggedInUser(t *testing.T) {
	setupDesktopAutoLoginTestDB(t)

	first := &model.User{
		Username: "older",
		Password: "pass1",
		Name:     "Older User",
		Provider: "password",
		Enabled:  true,
	}
	second := &model.User{
		Username: "newer",
		Password: "pass2",
		Name:     "Newer User",
		Provider: "password",
		Enabled:  true,
	}

	require.NoError(t, model.AddSuperUser(first))
	require.NoError(t, model.AddSuperUser(second))

	// Mark first user as recently logged in; desktop auto login should reuse it.
	require.NoError(t, model.LoginUser(first))

	selected, created, err := ensureDesktopAutoLoginUser()
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, selected)
	require.Equal(t, first.Username, selected.Username)
}

func TestEnsureDesktopAutoLoginUser_SkipsDisabledUsers(t *testing.T) {
	setupDesktopAutoLoginTestDB(t)

	disabled := &model.User{
		Username: "disabled-user",
		Password: "pass3",
		Name:     "Disabled User",
		Provider: "password",
		Enabled:  true,
	}
	enabled := &model.User{
		Username: "enabled-user",
		Password: "pass4",
		Name:     "Enabled User",
		Provider: "password",
		Enabled:  true,
	}

	require.NoError(t, model.AddSuperUser(disabled))
	require.NoError(t, model.AddSuperUser(enabled))
	require.NoError(t, model.SetUserEnabled(disabled.ID, false))

	selected, created, err := ensureDesktopAutoLoginUser()
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, selected)
	require.Equal(t, enabled.Username, selected.Username)
}

func TestEnsureDesktopAutoLoginUser_DisabledDefaultUsernameUsesFallback(t *testing.T) {
	setupDesktopAutoLoginTestDB(t)

	disabledDefault := &model.User{
		Username: "admin",
		Password: "pass5",
		Name:     "Disabled Desktop",
		Provider: "password",
		Enabled:  true,
	}
	require.NoError(t, model.AddSuperUser(disabledDefault))
	require.NoError(t, model.SetUserEnabled(disabledDefault.ID, false))

	selected, created, err := ensureDesktopAutoLoginUser()
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, selected)
	require.NotEqual(t, "admin", selected.Username)
	require.Equal(t, "Admin", selected.Name)
	require.True(t, selected.Enabled)
}
