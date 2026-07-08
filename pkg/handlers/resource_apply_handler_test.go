package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResolveApplyResourceMetadataUsesCanonicalPluralNames(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		want      string
		wantScope bool
	}{
		{name: "ingress", kind: "Ingress", want: "ingresses"},
		{name: "network policy", kind: "NetworkPolicy", want: "networkpolicies"},
		{name: "config map", kind: "ConfigMap", want: "configmaps"},
		{name: "node", kind: "Node", want: "nodes", wantScope: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := applyTestObject("networking.k8s.io/v1", tt.kind, "example", "team-a")
			meta, err := resolveApplyResourceMetadata(nil, obj)

			require.NoError(t, err)
			require.Equal(t, tt.want, meta.Resource)
			require.Equal(t, tt.wantScope, meta.ClusterScoped)
		})
	}
}

func TestFindApplyResourceMetadataUsesDiscoveryPluralAndScope(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "FooPolicy",
	}
	meta, ok := findApplyResourceMetadata(gvk, []metav1.APIResource{
		{Name: "foopolicies/status", Kind: "FooPolicy", Namespaced: true},
		{Name: "foopolicies", Kind: "FooPolicy", Namespaced: true},
	})

	require.True(t, ok)
	require.Equal(t, "foopolicies", meta.Resource)
	require.False(t, meta.ClusterScoped)
}

func TestResolveApplyResourceMetadataRejectsUnknownResourcesWithoutDiscovery(t *testing.T) {
	obj := applyTestObject("example.com/v1", "FooPolicy", "example", "team-a")
	meta, err := resolveApplyResourceMetadata(nil, obj)

	require.Error(t, err)
	require.Empty(t, meta.Resource)
	require.NotContains(t, err.Error(), "foopolicys")
}

func TestNormalizeApplyResourceNamespaceDefaultsToWorkspaceScope(t *testing.T) {
	cs := &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"}
	obj := applyTestObject("v1", "ConfigMap", "example", "")

	namespace, err := normalizeApplyResourceNamespace(cs, applyResourceMetadata{Resource: string(common.ConfigMaps)}, obj)

	require.NoError(t, err)
	require.Equal(t, "team-a", namespace)
	require.Equal(t, "team-a", obj.GetNamespace())
}

func TestNormalizeApplyResourceNamespaceDefaultsAllNamespacesToWorkspaceScope(t *testing.T) {
	cs := &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"}
	obj := applyTestObject("v1", "ConfigMap", "example", common.AllNamespaces)

	namespace, err := normalizeApplyResourceNamespace(cs, applyResourceMetadata{Resource: string(common.ConfigMaps)}, obj)

	require.NoError(t, err)
	require.Equal(t, "team-a", namespace)
	require.Equal(t, "team-a", obj.GetNamespace())
}

func TestNormalizeApplyResourceNamespaceRejectsOutsideWorkspaceScope(t *testing.T) {
	cs := &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"}
	obj := applyTestObject("v1", "ConfigMap", "example", "team-b")

	namespace, err := normalizeApplyResourceNamespace(cs, applyResourceMetadata{Resource: string(common.ConfigMaps)}, obj)

	require.Error(t, err)
	require.Empty(t, namespace)
	require.Contains(t, err.Error(), "outside the current workspace scope")
}

func TestNormalizeApplyResourceNamespaceRejectsClusterScopedInWorkspaceScope(t *testing.T) {
	cs := &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"}
	obj := applyTestObject("v1", "Node", "node-a", "")

	namespace, err := normalizeApplyResourceNamespace(cs, applyResourceMetadata{
		Resource:      string(common.Nodes),
		ClusterScoped: true,
	}, obj)

	require.Error(t, err)
	require.Empty(t, namespace)
	require.Contains(t, err.Error(), "cluster-scoped")
}

func applyTestObject(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}
