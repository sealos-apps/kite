package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
)

const (
	ClusterNameHeader = "x-cluster-name"
	ClusterNameKey    = "cluster-name"
	K8sClientKey      = "k8s-client"
	PromClientKey     = "prom-client"
)

// ClusterMiddleware extracts cluster name from header and injects clients into context
func ClusterMiddleware(cm *cluster.ClusterManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.GetHeader(ClusterNameHeader)
		if clusterName == "" {
			if v, ok := c.GetQuery(ClusterNameHeader); ok {
				clusterName = v
			}
			if clusterName == "" {
				clusterName, _ = c.Cookie(ClusterNameHeader)
			}
		}
		userValue, hasUser := c.Get("user")
		if !hasUser {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user context not found"})
			c.Abort()
			return
		}

		user, ok := userValue.(model.User)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user context"})
			c.Abort()
			return
		}

		clientSet, err := cm.ResolveClientSetForUser(user, clusterName)
		if err != nil {
			statusCode := http.StatusNotFound
			if errors.Is(err, cluster.ErrClusterAccessDenied) || errors.Is(err, cluster.ErrNoAccessibleCluster) {
				statusCode = http.StatusForbidden
			}
			c.JSON(statusCode, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Set("cluster", clientSet)
		c.Set(ClusterNameKey, clientSet.Name)
		c.Next()
	}
}
