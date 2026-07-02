package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
)

func TestHelmMutationToolsRequireConfirmation(t *testing.T) {
	for _, name := range []string{
		"install_helm_release",
		"upgrade_helm_release",
		"rollback_helm_release",
		"uninstall_helm_release",
	} {
		if !MutationTools[name] {
			t.Fatalf("expected %s to require confirmation", name)
		}
	}
	for _, name := range []string{
		"list_helm_releases",
		"get_helm_release",
		"get_helm_release_history",
		"dry_run_install_helm_release",
		"dry_run_upgrade_helm_release",
	} {
		if MutationTools[name] {
			t.Fatalf("did not expect %s to require confirmation", name)
		}
	}
}

func TestParseHelmInstallToolRequestUsesNamespaceScope(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})
	common.NamespaceScopeExemptNamespaces = map[string]struct{}{}

	cs := &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"}
	req, err := parseHelmInstallToolRequest(cs, map[string]interface{}{
		"release_name":    "api",
		"namespace":       common.AllNamespaces,
		"repository_name": "stable",
		"chart_name":      "nginx",
		"values": map[string]interface{}{
			"replicaCount": float64(2),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Namespace != "team-a" {
		t.Fatalf("expected scoped namespace team-a, got %s", req.Namespace)
	}
	if req.Values["replicaCount"] != float64(2) {
		t.Fatalf("expected values to be preserved, got %#v", req.Values)
	}

	_, err = parseHelmInstallToolRequest(cs, map[string]interface{}{
		"release_name": "api",
		"namespace":    "team-b",
	})
	if err == nil {
		t.Fatalf("expected cross-namespace request to fail")
	}
}

func TestHelmDryRunResponseSummarizesResources(t *testing.T) {
	rel := &release.Release{
		Name:      "api",
		Namespace: "team-a",
		Version:   3,
		Chart: &chart.Chart{Metadata: &chart.Metadata{
			Name:       "nginx",
			Version:    "1.2.3",
			AppVersion: "1.25",
		}},
		Manifest: strings.TrimSpace(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: api-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: team-a
`),
		Info: &release.Info{
			Status:       releasecommon.StatusDeployed,
			Description:  "dry run",
			LastDeployed: time.Now(),
		},
	}

	result := helmDryRunResponse(rel, "", helmutil.ImageCheckResult{})
	if result.ReleaseName != "api" || result.Namespace != "team-a" {
		t.Fatalf("unexpected release identity: %#v", result)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("expected 2 summarized resources, got %#v", result.Resources)
	}
	if result.Resources[0].Kind != "ConfigMap" || result.Resources[0].Namespace != "team-a" {
		t.Fatalf("expected default namespace on ConfigMap, got %#v", result.Resources[0])
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(data), "apiVersion:") {
		t.Fatalf("dry-run tool response should not include full manifest content: %s", data)
	}
}
