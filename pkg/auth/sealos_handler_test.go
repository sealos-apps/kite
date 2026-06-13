package auth

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

func Test_buildSealosClusterUpdates(t *testing.T) {
	t.Run("unchanged cluster has no updates", func(t *testing.T) {
		cluster := &model.Cluster{
			Description:   "Managed by Sealos SSO (namespace: ns-a)",
			Config:        model.SecretString("same-kubeconfig"),
			PrometheusURL: "http://prometheus",
			InCluster:     false,
			Enable:        true,
		}

		updates := buildSealosClusterUpdates(cluster, "Managed by Sealos SSO (namespace: ns-a)", "same-kubeconfig", "http://prometheus")

		assert.Empty(t, updates)
	})

	t.Run("changed kubeconfig is updated", func(t *testing.T) {
		cluster := &model.Cluster{
			Description:   "Managed by Sealos SSO",
			Config:        model.SecretString("old-kubeconfig"),
			PrometheusURL: "http://prometheus",
			InCluster:     false,
			Enable:        true,
		}

		updates := buildSealosClusterUpdates(cluster, "Managed by Sealos SSO", "new-kubeconfig", "http://prometheus")

		assert.Equal(t, model.SecretString("new-kubeconfig"), updates["config"])
		assert.Len(t, updates, 1)
	})

	t.Run("missing defaults and disabled flags are repaired", func(t *testing.T) {
		cluster := &model.Cluster{
			Description:   "old description",
			Config:        model.SecretString("same-kubeconfig"),
			PrometheusURL: " ",
			InCluster:     true,
			Enable:        false,
		}

		updates := buildSealosClusterUpdates(cluster, "Managed by Sealos SSO", "same-kubeconfig", "http://prometheus")

		assert.Equal(t, "Managed by Sealos SSO", updates["description"])
		assert.Equal(t, false, updates["in_cluster"])
		assert.Equal(t, true, updates["enable"])
		assert.Equal(t, "http://prometheus", updates["prometheus_url"])
		assert.NotContains(t, updates, "config")
	})
}

func Test_buildSealosRoleNamespaces(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})

	t.Run("empty namespace grants all namespaces", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{}
		assert.Equal(t, []string{"*"}, buildSealosRoleNamespaces(""))
	})

	t.Run("regular namespace is namespace scoped", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{}
		assert.Equal(t, []string{"default"}, buildSealosRoleNamespaces("default"))
	})

	t.Run("exempt namespace grants all namespaces", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{
			"ns-admin": {},
		}
		assert.Equal(t, []string{"*"}, buildSealosRoleNamespaces("ns-admin"))
	})

	t.Run("exempt namespace matching is case insensitive and trim aware", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{
			"ns-admin": {},
		}
		assert.Equal(t, []string{"*"}, buildSealosRoleNamespaces(" NS-ADMIN "))
	})
}

func TestEnsureSealosAdminRoleAssignmentIfExempt(t *testing.T) {
	useTestSealosAuthDB(t)

	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})

	require.NoError(t, model.InitDefaultRole())
	common.NamespaceScopeExemptNamespaces = map[string]struct{}{
		"ns-admin": {},
	}

	require.NoError(t, ensureSealosAdminRoleAssignmentIfExempt("ns-admin", "sealos-admin"))

	adminRole, err := model.GetRoleByName(model.DefaultAdminRole.Name)
	require.NoError(t, err)

	var adminAssignments int64
	require.NoError(t, model.DB.Model(&model.RoleAssignment{}).Where(
		"role_id = ? AND subject_type = ? AND subject = ?",
		adminRole.ID,
		model.SubjectTypeUser,
		"sealos-admin",
	).Count(&adminAssignments).Error)
	assert.EqualValues(t, 1, adminAssignments)

	require.NoError(t, ensureSealosAdminRoleAssignmentIfExempt("default", "sealos-regular"))

	var regularAssignments int64
	require.NoError(t, model.DB.Model(&model.RoleAssignment{}).Where(
		"subject = ?",
		"sealos-regular",
	).Count(&regularAssignments).Error)
	assert.EqualValues(t, 0, regularAssignments)
}

func useTestSealosAuthDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.RoleAssignment{}))
	model.DB = db

	t.Cleanup(func() {
		model.DB = originalDB
	})
}
