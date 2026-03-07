package permissions

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const ClusterNameHeader = "x-cluster-name"

func ClusterNameFromRequest(c *gin.Context) string {
	clusterName := strings.TrimSpace(c.GetHeader(ClusterNameHeader))
	if clusterName != "" {
		return clusterName
	}
	if queryCluster := strings.TrimSpace(c.Query(ClusterNameHeader)); queryCluster != "" {
		return queryCluster
	}
	if cookieCluster, err := c.Cookie(ClusterNameHeader); err == nil {
		return strings.TrimSpace(cookieCluster)
	}
	return ""
}
