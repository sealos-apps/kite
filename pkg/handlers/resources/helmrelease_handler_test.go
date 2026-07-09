package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
)

func TestValidateHelmReleaseAutoUpgradeRejectsDisabledArtifactHub(t *testing.T) {
	original := common.HelmArtifactHubEnabled
	common.HelmArtifactHubEnabled = false
	defer func() {
		common.HelmArtifactHubEnabled = original
	}()

	err := validateHelmReleaseAutoUpgradeRequest(helmReleaseAutoUpgradeRequest{
		Enabled:           true,
		Source:            helmutil.ChartSourceArtifactHub,
		RepositoryName:    "bitnami",
		ChartName:         "nginx",
		ScheduleType:      model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes:   60,
		ScheduleTime:      "03:00",
		TimeoutMinutes:    5,
		RollbackOnFailure: true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Artifact Hub chart source is disabled")
}

func TestValidateHelmReleaseAutoUpgradeAllowsOCIWhenArtifactHubDisabled(t *testing.T) {
	original := common.HelmArtifactHubEnabled
	common.HelmArtifactHubEnabled = false
	defer func() {
		common.HelmArtifactHubEnabled = original
	}()

	err := validateHelmReleaseAutoUpgradeRequest(helmReleaseAutoUpgradeRequest{
		Enabled:           true,
		Source:            helmutil.ChartSourceOCI,
		RepositoryName:    "offline",
		ChartName:         "nginx",
		ScheduleType:      model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes:   60,
		ScheduleTime:      "03:00",
		TimeoutMinutes:    5,
		RollbackOnFailure: true,
	})
	require.NoError(t, err)
}

func TestHelmReleaseResolveNamespaceUsesWorkspaceScopeForAllNamespaces(t *testing.T) {
	c := newHelmReleaseNamespaceContext(t, &cluster.ClientSet{
		NamespaceScoped: true,
		Namespace:       "team-a",
	})

	namespace, err := NewHelmReleaseHandler().resolveNamespace(c, common.AllNamespaces, false)

	require.NoError(t, err)
	require.Equal(t, "team-a", namespace)
}

func TestHelmReleaseResolveNamespaceRejectsOutsideWorkspaceScope(t *testing.T) {
	c := newHelmReleaseNamespaceContext(t, &cluster.ClientSet{
		NamespaceScoped: true,
		Namespace:       "team-a",
	})

	namespace, err := NewHelmReleaseHandler().resolveNamespace(c, "team-b", true)

	require.Empty(t, namespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside the current workspace scope team-a")
	require.Equal(t, http.StatusForbidden, helmReleaseErrorStatus(err))
}

func TestHelmReleaseResolveNamespaceKeepsAllNamespacesForUnscopedList(t *testing.T) {
	c := newHelmReleaseNamespaceContext(t, &cluster.ClientSet{})

	namespace, err := NewHelmReleaseHandler().resolveNamespace(c, common.AllNamespaces, false)

	require.NoError(t, err)
	require.Equal(t, common.AllNamespaces, namespace)
}

func TestHelmReleaseResolveNamespaceRequiresNamespaceForUnscopedGet(t *testing.T) {
	c := newHelmReleaseNamespaceContext(t, &cluster.ClientSet{})

	namespace, err := NewHelmReleaseHandler().resolveNamespace(c, common.AllNamespaces, true)

	require.Empty(t, namespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "namespace is required")
	require.Equal(t, http.StatusBadRequest, helmReleaseErrorStatus(err))
}

func newHelmReleaseNamespaceContext(t *testing.T, cs *cluster.ClientSet) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("cluster", cs)
	return c
}
