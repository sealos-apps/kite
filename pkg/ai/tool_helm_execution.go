package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmguard"
	"github.com/zxh326/kite/pkg/helmutil"
	pkgmodel "github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"github.com/zxh326/kite/pkg/scheduler"
	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	release "helm.sh/helm/v4/pkg/release/v1"
)

const aiHelmActionTimeout = 5 * time.Minute

type helmReleaseToolResponse struct {
	ReleaseName  string                            `json:"releaseName,omitempty"`
	Namespace    string                            `json:"namespace,omitempty"`
	Status       string                            `json:"status,omitempty"`
	Revision     int                               `json:"revision,omitempty"`
	Chart        string                            `json:"chart,omitempty"`
	ChartName    string                            `json:"chartName,omitempty"`
	ChartVersion string                            `json:"chartVersion,omitempty"`
	AppVersion   string                            `json:"appVersion,omitempty"`
	Description  string                            `json:"description,omitempty"`
	Resources    []helmReleaseToolResource         `json:"resources,omitempty"`
	History      []helmutil.HelmReleaseHistoryItem `json:"history,omitempty"`
	Message      string                            `json:"message,omitempty"`
}

type helmReleaseToolResource struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     string `json:"status,omitempty"`
}

type helmInstallToolRequest struct {
	ReleaseName     string
	Namespace       string
	Source          string
	RepositoryName  string
	ChartName       string
	ChartVersion    string
	ChartURL        string
	Values          map[string]interface{}
	Description     string
	CreateNamespace bool
	Wait            bool
}

type helmUpgradeToolRequest struct {
	ReleaseName       string
	Namespace         string
	Source            string
	RepositoryName    string
	ChartName         string
	ChartVersion      string
	ChartURL          string
	Values            map[string]interface{}
	Description       string
	ForceConflicts    bool
	RollbackOnFailure bool
	Wait              bool
}

func executeListHelmReleases(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	namespace, _ := args["namespace"].(string)
	namespace, err := scopedNamespaceForTool(cs, helmReleaseResourceInfo(), namespace)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	allNamespaces := namespace == common.AllNamespaces
	releases, err := helmutil.ListReleases(cfg, allNamespaces)
	if err != nil {
		return fmt.Sprintf("Error listing Helm releases: %v", err), true
	}

	user, _ := currentUserFromGin(c)
	items := make([]helmReleaseToolResponse, 0, len(releases))
	for _, rel := range releases {
		if rel == nil {
			continue
		}
		if allNamespaces && !rbac.CanAccess(user, string(common.HelmReleases), string(common.VerbGet), cs.Name, rel.Namespace) {
			continue
		}
		items = append(items, helmReleaseSummary(rel, false))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace == items[j].Namespace {
			return items[i].ReleaseName < items[j].ReleaseName
		}
		return items[i].Namespace < items[j].Namespace
	})

	return marshalToolResult(map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

func executeGetHelmRelease(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	rel, err := helmutil.GetRelease(cfg, releaseName)
	if err != nil {
		return fmt.Sprintf("Error getting Helm release %s/%s: %v", namespace, releaseName, err), true
	}
	return marshalToolResult(helmReleaseSummary(rel, true))
}

func executeGetHelmReleaseHistory(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	items, err := helmutil.ReleaseHistoryItems(cfg, releaseName)
	if err != nil {
		return fmt.Sprintf("Error getting Helm release history for %s/%s: %v", namespace, releaseName, err), true
	}
	return marshalToolResult(helmReleaseToolResponse{
		ReleaseName: releaseName,
		Namespace:   namespace,
		History:     items,
	})
}

func executeInstallHelmRelease(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, user pkgmodel.User, args map[string]interface{}, dryRun bool) (string, bool) {
	req, err := parseHelmInstallToolRequest(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	chartPackage, loadedChart, err := loadHelmChart(ctx, req.Source, req.RepositoryName, req.ChartName, req.ChartVersion, req.ChartURL)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(req.Namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	description := req.Description
	if description == "" {
		if dryRun {
			description = "Dry run install requested from Kite AI"
		} else {
			description = "Install requested from Kite AI"
		}
	}
	opts := helmutil.InstallReleaseOptions{
		ReleaseName:     req.ReleaseName,
		Namespace:       req.Namespace,
		Timeout:         aiHelmActionTimeout,
		Description:     description,
		CreateNamespace: req.CreateNamespace,
		DryRun:          dryRun,
		Wait:            req.Wait,
	}
	preview, err := helmutil.DryRunInstallRelease(ctx, cfg, loadedChart, req.Values, opts)
	if err != nil {
		return fmt.Sprintf("Error rendering Helm install for %s/%s: %v", req.Namespace, req.ReleaseName, err), true
	}
	if err := helmguard.AuthorizeCreateNamespace(user, cs, req.CreateNamespace); err != nil {
		return "Forbidden: " + err.Error(), true
	}
	if err := helmguard.AuthorizeRelease(ctx, user, cs, preview, string(common.VerbCreate)); err != nil {
		return "Forbidden: " + err.Error(), true
	}
	if dryRun {
		return marshalToolResult(helmDryRunResponse(preview, chartPackage.Version))
	}

	var rel *release.Release
	var runErr error
	defer func() {
		helmutil.RecordReleaseHistory(cs.Name, user.ID, "ai", "install", req.ReleaseName, req.Namespace, nil, rel, runErr == nil, runErr)
	}()
	rel, runErr = helmutil.InstallRelease(ctx, cfg, loadedChart, req.Values, opts)
	if runErr != nil {
		return fmt.Sprintf("Error installing Helm release %s/%s: %v", req.Namespace, req.ReleaseName, runErr), true
	}
	return marshalToolResult(helmReleaseSummary(rel, true))
}

func executeUpgradeHelmRelease(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, user pkgmodel.User, args map[string]interface{}, dryRun bool) (string, bool) {
	req, err := parseHelmUpgradeToolRequest(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(req.Namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	current, err := helmutil.GetRelease(cfg, req.ReleaseName)
	if err != nil {
		return fmt.Sprintf("Error getting Helm release %s/%s: %v", req.Namespace, req.ReleaseName, err), true
	}
	if current.Chart == nil {
		return fmt.Sprintf("Error: Helm release %s/%s chart is missing", req.Namespace, req.ReleaseName), true
	}

	chartToUpgrade := current.Chart
	resolvedVersion := ""
	if req.ChartURL != "" || req.ChartName != "" || req.RepositoryName != "" || req.Source != "" || req.ChartVersion != "" {
		chartName := req.ChartName
		if chartName == "" && current.Chart.Metadata != nil {
			chartName = current.Chart.Metadata.Name
		}
		chartPackage, loadedChart, err := loadHelmChart(ctx, req.Source, req.RepositoryName, chartName, req.ChartVersion, req.ChartURL)
		if err != nil {
			return "Error: " + err.Error(), true
		}
		chartToUpgrade = loadedChart
		resolvedVersion = chartPackage.Version
	}
	description := req.Description
	if description == "" {
		if dryRun {
			description = "Dry run upgrade requested from Kite AI"
		} else {
			description = "Upgrade requested from Kite AI"
		}
	}
	opts := helmutil.UpgradeReleaseOptions{
		Namespace:         req.Namespace,
		Timeout:           aiHelmActionTimeout,
		ReuseValues:       req.Values == nil,
		Description:       description,
		ForceConflicts:    req.ForceConflicts,
		RollbackOnFailure: req.RollbackOnFailure,
		DryRun:            dryRun,
		Wait:              req.Wait,
	}
	values := req.Values
	if values == nil {
		values = map[string]interface{}{}
	}
	preview, err := helmutil.DryRunUpgradeRelease(ctx, cfg, req.ReleaseName, chartToUpgrade, values, opts)
	if err != nil {
		return fmt.Sprintf("Error rendering Helm upgrade for %s/%s: %v", req.Namespace, req.ReleaseName, err), true
	}
	if err := helmguard.AuthorizeReleaseChange(ctx, user, cs, current, preview); err != nil {
		return "Forbidden: " + err.Error(), true
	}
	if dryRun {
		response := helmDryRunDiffResponse(current, preview, resolvedVersion)
		return marshalToolResult(response)
	}

	var rel *release.Release
	var runErr error
	defer func() {
		helmutil.RecordReleaseHistory(cs.Name, user.ID, "ai", "upgrade", req.ReleaseName, req.Namespace, current, rel, runErr == nil, runErr)
	}()
	rel, runErr = helmutil.UpgradeRelease(ctx, cfg, req.ReleaseName, chartToUpgrade, values, opts)
	if runErr != nil {
		return fmt.Sprintf("Error upgrading Helm release %s/%s: %v", req.Namespace, req.ReleaseName, runErr), true
	}
	return marshalToolResult(helmReleaseSummary(rel, true))
}

func executeRollbackHelmRelease(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, user pkgmodel.User, args map[string]interface{}) (string, bool) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	current, err := helmutil.GetRelease(cfg, releaseName)
	if err != nil {
		return fmt.Sprintf("Error getting Helm release %s/%s: %v", namespace, releaseName, err), true
	}
	revision := getOptionalInt(args, "revision")
	if revision == 0 {
		revision = current.Version - 1
	}
	if revision <= 0 {
		return "Error: no previous Helm release revision found", true
	}
	target, err := helmutil.GetReleaseRevision(cfg, releaseName, revision)
	if err != nil {
		return fmt.Sprintf("Error getting target Helm revision %d for %s/%s: %v", revision, namespace, releaseName, err), true
	}
	if err := helmguard.AuthorizeReleaseChange(ctx, user, cs, current, target); err != nil {
		return "Forbidden: " + err.Error(), true
	}

	var next *release.Release
	var runErr error
	success := false
	defer func() {
		helmutil.RecordReleaseHistory(cs.Name, user.ID, "ai", "rollback", releaseName, namespace, current, next, success, runErr)
	}()
	runErr = helmutil.RollbackRelease(cfg, releaseName, helmutil.RollbackReleaseOptions{
		Version: revision,
		Timeout: aiHelmActionTimeout,
	})
	if runErr != nil {
		return fmt.Sprintf("Error rolling back Helm release %s/%s: %v", namespace, releaseName, runErr), true
	}
	success = true
	next = target
	next, err = helmutil.GetRelease(cfg, releaseName)
	if err != nil {
		return marshalToolResult(helmReleaseToolResponse{
			ReleaseName: releaseName,
			Namespace:   namespace,
			Message:     fmt.Sprintf("Helm release rolled back to revision %d, but verification read failed: %v", revision, err),
		})
	}
	return marshalToolResult(helmReleaseSummary(next, true))
}

func executeUninstallHelmRelease(ctx context.Context, c *gin.Context, cs *cluster.ClientSet, user pkgmodel.User, args map[string]interface{}) (string, bool) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return "Error: " + err.Error(), true
	}
	cfg, err := helmActionConfig(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return "Error: " + err.Error(), true
	}
	current, err := helmutil.GetRelease(cfg, releaseName)
	if err != nil {
		return fmt.Sprintf("Error getting Helm release %s/%s: %v", namespace, releaseName, err), true
	}
	if err := helmguard.AuthorizeReleaseDelete(ctx, user, cs, current); err != nil {
		return "Forbidden: " + err.Error(), true
	}
	description, _ := args["description"].(string)
	if strings.TrimSpace(description) == "" {
		description = "Uninstall requested from Kite AI"
	}

	success := false
	var runErr error
	defer func() {
		helmutil.RecordReleaseHistory(cs.Name, user.ID, "ai", "delete", releaseName, namespace, current, nil, success, runErr)
	}()
	runErr = helmutil.UninstallRelease(cfg, releaseName, helmutil.UninstallReleaseOptions{
		Timeout:     aiHelmActionTimeout,
		Description: strings.TrimSpace(description),
	})
	if runErr != nil {
		return fmt.Sprintf("Error uninstalling Helm release %s/%s: %v", namespace, releaseName, runErr), true
	}
	success = true
	message := "Helm release uninstalled"
	if err := deleteHelmReleaseAutoUpgradeTask(cs.Name, current.Namespace, current.Name); err != nil {
		message = fmt.Sprintf("Helm release uninstalled, but failed to delete auto-upgrade task: %v", err)
	}
	return marshalToolResult(helmReleaseToolResponse{
		ReleaseName: releaseName,
		Namespace:   namespace,
		Message:     message,
	})
}

func parseHelmInstallToolRequest(cs *cluster.ClientSet, args map[string]interface{}) (helmInstallToolRequest, error) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return helmInstallToolRequest{}, err
	}
	values, err := getOptionalValues(args)
	if err != nil {
		return helmInstallToolRequest{}, err
	}
	return helmInstallToolRequest{
		ReleaseName:     releaseName,
		Namespace:       namespace,
		Source:          getOptionalString(args, "source"),
		RepositoryName:  getOptionalString(args, "repository_name"),
		ChartName:       getOptionalString(args, "chart_name"),
		ChartVersion:    getOptionalString(args, "chart_version"),
		ChartURL:        getOptionalString(args, "chart_url"),
		Values:          values,
		Description:     getOptionalString(args, "description"),
		CreateNamespace: getOptionalBool(args, "create_namespace"),
		Wait:            getOptionalBool(args, "wait"),
	}, nil
}

func parseHelmUpgradeToolRequest(cs *cluster.ClientSet, args map[string]interface{}) (helmUpgradeToolRequest, error) {
	releaseName, namespace, err := requiredHelmReleaseNameAndNamespace(cs, args)
	if err != nil {
		return helmUpgradeToolRequest{}, err
	}
	values, err := getOptionalValues(args)
	if err != nil {
		return helmUpgradeToolRequest{}, err
	}
	return helmUpgradeToolRequest{
		ReleaseName:       releaseName,
		Namespace:         namespace,
		Source:            getOptionalString(args, "source"),
		RepositoryName:    getOptionalString(args, "repository_name"),
		ChartName:         getOptionalString(args, "chart_name"),
		ChartVersion:      getOptionalString(args, "chart_version"),
		ChartURL:          getOptionalString(args, "chart_url"),
		Values:            values,
		Description:       getOptionalString(args, "description"),
		ForceConflicts:    getOptionalBool(args, "force_conflicts"),
		RollbackOnFailure: getOptionalBool(args, "rollback_on_failure"),
		Wait:              getOptionalBool(args, "wait"),
	}, nil
}

func requiredHelmReleaseNameAndNamespace(cs *cluster.ClientSet, args map[string]interface{}) (string, string, error) {
	releaseName, err := getRequiredString(args, "release_name")
	if err != nil {
		return "", "", err
	}
	namespace, err := requiredHelmReleaseNamespace(cs, args)
	if err != nil {
		return "", "", err
	}
	if namespace == "" || namespace == common.AllNamespaces {
		return "", "", fmt.Errorf("namespace is required")
	}
	return releaseName, namespace, nil
}

func loadHelmChart(ctx context.Context, source, repositoryName, chartName, chartVersion, chartURL string) (helmutil.ChartPackage, *chart.Chart, error) {
	chartPackage, err := helmutil.ResolveChartPackage(ctx, helmutil.ChartSourceRef{
		Source:         strings.TrimSpace(source),
		RepositoryName: strings.TrimSpace(repositoryName),
		ChartName:      strings.TrimSpace(chartName),
		Version:        strings.TrimSpace(chartVersion),
		URL:            strings.TrimSpace(chartURL),
	})
	if err != nil {
		return helmutil.ChartPackage{}, nil, err
	}
	loadedChart, err := helmutil.LoadArchive(chartPackage.URL, chartPackage.Repository)
	if err != nil {
		return helmutil.ChartPackage{}, nil, err
	}
	return chartPackage, loadedChart, nil
}

func helmActionConfig(cs *cluster.ClientSet, namespace string) (*action.Configuration, error) {
	if cs == nil || cs.K8sClient == nil || cs.K8sClient.Configuration == nil {
		return nil, fmt.Errorf("cluster REST config is required")
	}
	return helmutil.NewActionConfig(cs.K8sClient.Configuration, namespace)
}

func helmReleaseSummary(rel *release.Release, details bool) helmReleaseToolResponse {
	hr := helmutil.ToHelmRelease(rel, details)
	response := helmReleaseToolResponse{
		ReleaseName:  hr.Spec.ReleaseName,
		Namespace:    hr.Spec.Namespace,
		Status:       hr.Status.Status,
		Revision:     hr.Spec.Revision,
		Chart:        hr.Spec.Chart,
		ChartName:    hr.Spec.ChartName,
		ChartVersion: hr.Spec.ChartVersion,
		AppVersion:   hr.Spec.AppVersion,
		Description:  hr.Spec.Description,
	}
	if details {
		response.Resources = helmToolResourcesFromReleaseResources(hr.Status.Resources)
	}
	return response
}

func helmDryRunResponse(rel *release.Release, resolvedVersion string) helmReleaseToolResponse {
	response := helmReleaseSummary(rel, false)
	if response.ChartVersion == "" {
		response.ChartVersion = resolvedVersion
	}
	preview := helmutil.ToHelmReleaseDryRunResponse(rel)
	response.Resources = helmToolResourcesFromDryRunResources(preview.Resources)
	response.Message = "Dry run rendered successfully; review resources before confirming install."
	return response
}

func helmDryRunDiffResponse(current, next *release.Release, resolvedVersion string) helmReleaseToolResponse {
	response := helmReleaseSummary(next, false)
	if response.ChartVersion == "" {
		response.ChartVersion = resolvedVersion
	}
	preview := helmutil.ToHelmReleaseDryRunDiffResponse(current, next)
	response.Resources = helmToolResourcesFromDryRunResources(preview.Resources)
	response.Message = "Dry run rendered successfully; review resource diff before confirming upgrade."
	return response
}

func helmToolResourcesFromReleaseResources(resources []helmutil.HelmReleaseResource) []helmReleaseToolResource {
	out := make([]helmReleaseToolResource, 0, len(resources))
	for _, resource := range resources {
		out = append(out, helmReleaseToolResource{
			APIVersion: resource.APIVersion,
			Kind:       resource.Kind,
			Name:       resource.Name,
			Namespace:  resource.Namespace,
		})
	}
	return out
}

func helmToolResourcesFromDryRunResources(resources []helmutil.HelmReleaseDryRunResource) []helmReleaseToolResource {
	out := make([]helmReleaseToolResource, 0, len(resources))
	for _, resource := range resources {
		out = append(out, helmReleaseToolResource{
			APIVersion: resource.APIVersion,
			Kind:       resource.Kind,
			Name:       resource.Name,
			Namespace:  resource.Namespace,
			Path:       resource.Path,
			Status:     resource.Status,
		})
	}
	return out
}

func getOptionalString(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func getOptionalBool(args map[string]interface{}, key string) bool {
	value, _ := args[key].(bool)
	return value
}

func getOptionalInt(args map[string]interface{}, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	default:
		return 0
	}
}

func getOptionalValues(args map[string]interface{}) (map[string]interface{}, error) {
	raw, ok := args["values"]
	if !ok || raw == nil {
		return nil, nil
	}
	values, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("values must be an object")
	}
	return values, nil
}

func marshalToolResult(value interface{}) (string, bool) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "Error: " + err.Error(), true
	}
	return string(data), false
}

func deleteHelmReleaseAutoUpgradeTask(clusterName, namespace, releaseName string) error {
	return pkgmodel.DB.
		Where("cluster_name = ? AND type = ? AND key = ?", clusterName, scheduler.HelmReleaseAutoUpgradeTaskType, scheduler.HelmReleaseAutoUpgradeTaskKey(namespace, releaseName)).
		Delete(&pkgmodel.ScheduledTask{}).Error
}
