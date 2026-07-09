package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
)

func RBACMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(model.User)
		cs := c.MustGet("cluster").(*cluster.ClientSet)

		verbs := method2verb(c.Request.Method)
		ns, resource := url2namespaceresource(c.Request.URL.Path)
		if ns == "" || resource == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid resource URL"})
			return
		}
		meta := common.LookupResource(resource)
		if meta != nil {
			resource = string(meta.Plural)
		}
		if namespaceScopeLocked(cs) {
			if meta != nil && meta.ClusterScoped {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "cluster-scoped resource " + resource + " is not available in namespace-scoped workspaces",
				})
				return
			}
			scopedNamespace := strings.TrimSpace(cs.Namespace)
			switch ns {
			case "", common.AllNamespaces:
				ns = scopedNamespace
			case scopedNamespace:
			default:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "namespace " + ns + " is outside the current workspace scope " + scopedNamespace,
				})
				return
			}
		}
		if resource == "namespaces" && verbs == "get" {
			// if user has roles, allow access to list namespaces resource
			// don't worry about security here, we will filter namespaces in the list namespace handler
			// this is just to allow users to list namespaces they have access to
			c.Next()
			return
		}

		canAccess := rbac.CanAccess(user, resource, verbs, cs.Name, ns)
		if canAccess {
			c.Next()
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": rbac.NoAccess(user.Key(), verbs, resource, ns, cs.Name)})
		}
	}
}

func namespaceScopeLocked(cs *cluster.ClientSet) bool {
	return cs != nil && cs.NamespaceScoped && strings.TrimSpace(cs.Namespace) != "" &&
		!common.IsNamespaceScopeExempt(cs.Namespace)
}

func method2verb(method string) string {
	switch method {
	case http.MethodPost:
		return string(common.VerbCreate)
	case http.MethodPut, http.MethodPatch:
		return string(common.VerbUpdate)
	default:
		return strings.ToLower(method)
	}
}

// url2namespaceresource converts a URL path to a resource type.
// For example:
//
// - /api/v1/pods/default/pods => default, pods
// - /api/v1/pvs/_all/some-pv => _all, some-pv
// - /api/v1/pods/default => default, pods
// - /api/v1/pods => "", pods
func url2namespaceresource(url string) (namespace string, resource string) {
	// Split the URL into its components
	parts := strings.Split(url, "/")
	if len(parts) < 4 {
		return
	}
	resource = parts[3] // The resource type is always the third part
	if len(parts) > 4 {
		namespace = parts[4]
	} else {
		namespace = "_all" // All namespaces
	}
	return
}
