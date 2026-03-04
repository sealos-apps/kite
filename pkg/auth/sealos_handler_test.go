package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zxh326/kite/pkg/common"
)

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
