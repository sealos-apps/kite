package resources

import (
	"testing"

	"github.com/stretchr/testify/require"
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
