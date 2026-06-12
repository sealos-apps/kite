package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmguard"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	release "helm.sh/helm/v4/pkg/release/v1"
)

const (
	HelmReleaseAutoUpgradeTaskType = "helm_release_auto_upgrade"
)

type HelmReleaseAutoUpgradePayload struct {
	Namespace         string     `json:"namespace"`
	ResourceType      string     `json:"resourceType"`
	ResourceName      string     `json:"resourceName"`
	Source            string     `json:"source"`
	RepositoryName    string     `json:"repositoryName"`
	ChartName         string     `json:"chartName"`
	TimeoutMinutes    int        `json:"timeoutMinutes"`
	RollbackOnFailure bool       `json:"rollbackOnFailure"`
	LastUpgradedAt    *time.Time `json:"lastUpgradedAt,omitempty"`
}

type helmReleaseAutoUpgradeExecutor struct {
	cm *cluster.ClusterManager
}

func registerHelmReleaseAutoUpgradeExecutor(manager *Manager, cm *cluster.ClusterManager) {
	manager.Register(HelmReleaseAutoUpgradeTaskType, &helmReleaseAutoUpgradeExecutor{cm: cm})
}

func (e *helmReleaseAutoUpgradeExecutor) Run(ctx context.Context, task model.ScheduledTask) error {
	var payload HelmReleaseAutoUpgradePayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return err
	}
	if task.CreatorID == 0 {
		return fmt.Errorf("scheduled task creator is missing")
	}
	creator, err := model.GetUserByID(uint64(task.CreatorID))
	if err != nil {
		return err
	}
	if creator == nil || !creator.Enabled {
		return fmt.Errorf("scheduled task creator is disabled or missing")
	}
	releaseName := payload.ResourceName
	cs, err := e.cm.GetClientSet(task.ClusterName)
	if err != nil {
		return err
	}
	if !rbac.CanAccess(*creator, string(common.HelmReleases), string(common.VerbUpdate), cs.Name, payload.Namespace) {
		return fmt.Errorf("%s", rbac.NoAccess(creator.Key(), string(common.VerbUpdate), string(common.HelmReleases), payload.Namespace, cs.Name))
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, helmutil.StorageNamespace(payload.Namespace))
	if err != nil {
		return err
	}
	current, err := helmutil.GetRelease(cfg, releaseName)
	if err != nil {
		return err
	}
	if current.Chart == nil {
		return fmt.Errorf("helm release chart is missing")
	}

	_, currentVersion, _ := helmutil.ChartInfo(current)
	nextChart, err := helmutil.LatestChartPackage(ctx, payload.Source, payload.RepositoryName, payload.ChartName)
	if err != nil {
		return err
	}
	if !helmutil.IsChartVersionNewer(nextChart.Version, currentVersion) {
		return nil
	}

	loadedChart, err := helmutil.LoadArchive(nextChart.URL, nextChart.Repository)
	if err != nil {
		return err
	}
	opts := helmutil.UpgradeReleaseOptions{
		Namespace:            payload.Namespace,
		Timeout:              time.Duration(payload.TimeoutMinutes) * time.Minute,
		ResetThenReuseValues: true,
		Description:          "Auto upgrade requested from Kite",
		RollbackOnFailure:    payload.RollbackOnFailure,
	}
	preview, err := helmutil.DryRunUpgradeRelease(ctx, cfg, releaseName, loadedChart, map[string]interface{}{}, opts)
	if err != nil {
		return err
	}
	if err := helmguard.AuthorizeReleaseChange(ctx, *creator, cs, current, preview); err != nil {
		return err
	}

	var next *release.Release
	var runErr error
	success := false
	defer func() {
		helmutil.RecordReleaseHistory(
			cs.Name,
			task.CreatorID,
			"auto",
			"upgrade",
			releaseName,
			payload.Namespace,
			current,
			next,
			success,
			runErr,
		)
	}()

	next, err = helmutil.UpgradeRelease(ctx, cfg, releaseName, loadedChart, map[string]interface{}{}, opts)
	if err != nil {
		runErr = err
		return err
	}
	success = true
	upgradedAt := time.Now()
	payload.LastUpgradedAt = &upgradedAt
	return saveHelmAutoUpgradePayload(task.ID, payload)
}

func saveHelmAutoUpgradePayload(taskID uint, payload HelmReleaseAutoUpgradePayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return model.DB.Model(&model.ScheduledTask{}).Where("id = ?", taskID).Update("payload", string(data)).Error
}

func HelmReleaseAutoUpgradeTaskKey(namespace, releaseName string) string {
	return namespace + "/" + releaseName
}

func HelmReleaseAutoUpgradeTaskName(namespace, releaseName string) string {
	return fmt.Sprintf("Helm release auto upgrade %s/%s", namespace, releaseName)
}
