package permissions

import (
	"strings"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/klog/v2"
)

// UserCapabilities contains user capability flags used by frontend behavior gates.
type UserCapabilities struct {
	CanCreateCustomCRDGroup bool `json:"canCreateCustomCRDGroup"`
	AIEnabled               bool `json:"aiEnabled"`
	KubectlEnabled          bool `json:"kubectlEnabled"`
}

// CanCreateCustomCRDGroup returns whether the user can create sidebar custom CRD groups.
// The permission model treats global admin and cluster-level CRD creators as equivalent.
func CanCreateCustomCRDGroup(user model.User, clusterName string) bool {
	if rbac.UserHasRole(user, model.DefaultAdminRole.Name) {
		return true
	}

	if clusterName != "" && rbac.CanAccess(user, "crds", string(common.VerbCreate), clusterName, "_all") {
		return true
	}
	if clusterName != "" {
		return false
	}

	// Fallback when cluster is unknown (for example, legacy clients missing cluster header/cookie).
	for _, role := range rbac.GetUserRoles(user) {
		for _, cluster := range role.Clusters {
			cluster = strings.TrimSpace(cluster)
			if cluster == "" || strings.HasPrefix(cluster, "!") {
				continue
			}
			if rbac.CanAccess(user, "crds", string(common.VerbCreate), cluster, "_all") {
				return true
			}
		}
	}
	return false
}

func BuildUserCapabilities(user model.User, clusterName string) UserCapabilities {
	capabilities := UserCapabilities{
		CanCreateCustomCRDGroup: CanCreateCustomCRDGroup(user, clusterName),
		AIEnabled:               false,
		KubectlEnabled:          false,
	}

	if model.DB == nil {
		return capabilities
	}

	setting, err := model.GetGeneralSetting()
	if err != nil {
		klog.Warningf("failed to load general setting for user capabilities: %v", err)
		return capabilities
	}

	capabilities.AIEnabled = setting.AIAgentEnabled
	capabilities.KubectlEnabled = setting.KubectlEnabled
	return capabilities
}
