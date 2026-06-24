package ai

import (
	"testing"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveStaticResourceInfoCRD(t *testing.T) {
	info := resolveStaticResourceInfo("crds")
	if info.Kind != "CustomResourceDefinition" {
		t.Fatalf("unexpected kind: %s", info.Kind)
	}
	if info.Resource != "customresourcedefinitions" {
		t.Fatalf("unexpected resource: %s", info.Resource)
	}
	if info.Group != "apiextensions.k8s.io" {
		t.Fatalf("unexpected group: %s", info.Group)
	}
	if info.Version != "v1" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if !info.ClusterScoped {
		t.Fatalf("expected cluster scoped")
	}
}

func TestResourceMatchesInputCRDVariants(t *testing.T) {
	resource := metav1.APIResource{
		Name:         "customresourcedefinitions",
		SingularName: "customresourcedefinition",
		Namespaced:   false,
		Kind:         "CustomResourceDefinition",
		ShortNames:   []string{"crd"},
	}

	cases := []string{
		"crd",
		"crds",
		"customresourcedefinition",
		"customresourcedefinitions",
		"customresourcedefinition.apiextensions.k8s.io",
		"customresourcedefinitions.apiextensions.k8s.io",
		"crd.apiextensions.k8s.io",
		"crds.apiextensions.k8s.io",
	}

	for _, input := range cases {
		if !resourceMatchesInput(input, "apiextensions.k8s.io", resource) {
			t.Fatalf("expected match for input %s", input)
		}
	}

	if resourceMatchesInput("crd.apps", "apiextensions.k8s.io", resource) {
		t.Fatalf("expected no match for crd.apps")
	}
}

func TestToolDefinitionsPrometheusToggle(t *testing.T) {
	hasPrometheusTool := func(defs []agentToolDefinition) bool {
		for _, def := range defs {
			if def.Name == "query_prometheus" {
				return true
			}
		}
		return false
	}

	if got := hasPrometheusTool(toolDefinitions(nil)); got {
		t.Fatalf("expected no Prometheus tool when client is absent")
	}

	if got := hasPrometheusTool(toolDefinitions(&cluster.ClientSet{PromClient: &prometheus.Client{}})); !got {
		t.Fatalf("expected Prometheus tool when client is present")
	}

	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})
	common.NamespaceScopeExemptNamespaces = map[string]struct{}{}
	if got := hasPrometheusTool(toolDefinitions(&cluster.ClientSet{
		NamespaceScoped: true,
		Namespace:       "team-a",
		PromClient:      &prometheus.Client{},
	})); got {
		t.Fatalf("expected no Prometheus tool in namespace-scoped workspace")
	}
}

func TestToolDefinitionsIncludeHelmTools(t *testing.T) {
	defs := toolDefinitions(&cluster.ClientSet{})
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
	}

	for _, name := range []string{
		"list_helm_releases",
		"get_helm_release",
		"get_helm_release_history",
		"dry_run_install_helm_release",
		"install_helm_release",
		"dry_run_upgrade_helm_release",
		"upgrade_helm_release",
		"rollback_helm_release",
		"uninstall_helm_release",
	} {
		if !names[name] {
			t.Fatalf("expected Helm tool %s to be defined", name)
		}
	}
}
