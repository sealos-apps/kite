package ai

import (
	"context"
	"reflect"
	"testing"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
)

func TestScopedNamespaceForTool(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})
	common.NamespaceScopeExemptNamespaces = map[string]struct{}{}

	tests := []struct {
		name      string
		cs        *cluster.ClientSet
		resource  resourceInfo
		namespace string
		want      string
		wantErr   bool
	}{
		{
			name:      "cluster scoped ignores namespace",
			resource:  resourceInfo{ClusterScoped: true},
			namespace: "default",
			want:      "",
		},
		{
			name:      "namespaced empty becomes all",
			resource:  resourceInfo{},
			namespace: " ",
			want:      "_all",
		},
		{
			name:      "namespaced passes through",
			resource:  resourceInfo{},
			namespace: " default ",
			want:      "default",
		},
		{
			name:      "namespace-scoped empty defaults to current namespace",
			cs:        &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"},
			resource:  resourceInfo{},
			namespace: " ",
			want:      "team-a",
		},
		{
			name:      "namespace-scoped all defaults to current namespace",
			cs:        &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"},
			resource:  resourceInfo{},
			namespace: common.AllNamespaces,
			want:      "team-a",
		},
		{
			name:      "namespace-scoped rejects other namespace",
			cs:        &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"},
			resource:  resourceInfo{},
			namespace: "team-b",
			wantErr:   true,
		},
		{
			name:      "namespace-scoped rejects cluster-scoped resource",
			cs:        &cluster.ClientSet{NamespaceScoped: true, Namespace: "team-a"},
			resource:  resourceInfo{Resource: "nodes", ClusterScoped: true},
			namespace: "",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scopedNamespaceForTool(tc.cs, tc.resource, tc.namespace)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected namespace: want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRequiredToolPermissions(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     []toolPermission
	}{
		{
			name:     "get resource",
			toolName: "get_resource",
			args: map[string]interface{}{
				"kind":      "pods",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "pods", Verb: "get", Namespace: "default"}},
		},
		{
			name:     "list resources across namespaces",
			toolName: "list_resources",
			args: map[string]interface{}{
				"kind": "Deployment",
			},
			want: []toolPermission{{Resource: "deployments", Verb: "get", Namespace: "_all"}},
		},
		{
			name:     "get pod logs",
			toolName: "get_pod_logs",
			args: map[string]interface{}{
				"name":      "nginx",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "pods", Verb: "log", Namespace: "default"}},
		},
		{
			name:     "cluster overview",
			toolName: "get_cluster_overview",
			args:     map[string]interface{}{},
			want: []toolPermission{
				{Resource: "nodes", Verb: "get", Namespace: ""},
				{Resource: "pods", Verb: "get", Namespace: "_all"},
				{Resource: "namespaces", Verb: "get", Namespace: ""},
				{Resource: "services", Verb: "get", Namespace: "_all"},
			},
		},
		{
			name:     "create namespaced resource",
			toolName: "create_resource",
			args: map[string]interface{}{
				"yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\n  namespace: default\n",
			},
			want: []toolPermission{{Resource: "configmaps", Verb: "create", Namespace: "default"}},
		},
		{
			name:     "create cluster scoped resource",
			toolName: "create_resource",
			args: map[string]interface{}{
				"yaml": "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: example\n",
			},
			want: []toolPermission{{Resource: "namespaces", Verb: "create", Namespace: ""}},
		},
		{
			name:     "patch cluster scoped resource",
			toolName: "patch_resource",
			args: map[string]interface{}{
				"kind":      "Node",
				"name":      "node-1",
				"namespace": "default",
				"patch":     `{"metadata":{"labels":{"env":"prod"}}}`,
			},
			want: []toolPermission{{Resource: "nodes", Verb: "update", Namespace: ""}},
		},
		{
			name:     "delete resource",
			toolName: "delete_resource",
			args: map[string]interface{}{
				"kind":      "Service",
				"name":      "api",
				"namespace": "default",
			},
			want: []toolPermission{{Resource: "services", Verb: "delete", Namespace: "default"}},
		},
		{
			name:     "prometheus query",
			toolName: "query_prometheus",
			args:     map[string]interface{}{"query": "up"},
			want:     []toolPermission{{Resource: "pods", Verb: "get", Namespace: "_all"}},
		},
		{
			name:     "list helm releases across namespaces",
			toolName: "list_helm_releases",
			args:     map[string]interface{}{},
			want:     []toolPermission{{Resource: "helmreleases", Verb: "get", Namespace: "_all"}},
		},
		{
			name:     "dry-run install helm release",
			toolName: "dry_run_install_helm_release",
			args: map[string]interface{}{
				"release_name": "api",
				"namespace":    "default",
			},
			want: []toolPermission{{Resource: "helmreleases", Verb: "create", Namespace: "default"}},
		},
		{
			name:     "get helm release",
			toolName: "get_helm_release",
			args: map[string]interface{}{
				"release_name": "api",
				"namespace":    "default",
			},
			want: []toolPermission{{Resource: "helmreleases", Verb: "get", Namespace: "default"}},
		},
		{
			name:     "install helm release",
			toolName: "install_helm_release",
			args: map[string]interface{}{
				"release_name": "api",
				"namespace":    "default",
			},
			want: []toolPermission{{Resource: "helmreleases", Verb: "create", Namespace: "default"}},
		},
		{
			name:     "dry-run upgrade helm release",
			toolName: "dry_run_upgrade_helm_release",
			args: map[string]interface{}{
				"release_name": "api",
				"namespace":    "default",
			},
			want: []toolPermission{{Resource: "helmreleases", Verb: "update", Namespace: "default"}},
		},
		{
			name:     "uninstall helm release",
			toolName: "uninstall_helm_release",
			args: map[string]interface{}{
				"release_name": "api",
				"namespace":    "default",
			},
			want: []toolPermission{{Resource: "helmreleases", Verb: "delete", Namespace: "default"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requiredToolPermissions(context.Background(), &cluster.ClientSet{}, tc.toolName, tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected permissions:\nwant: %#v\ngot:  %#v", tc.want, got)
			}
		})
	}
}

func TestRequiredToolPermissionsNamespaceScoped(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})
	common.NamespaceScopeExemptNamespaces = map[string]struct{}{}

	cs := &cluster.ClientSet{Name: "cluster-a", NamespaceScoped: true, Namespace: "team-a"}

	got, err := requiredToolPermissions(context.Background(), cs, "list_resources", map[string]interface{}{
		"kind": "Deployment",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []toolPermission{{Resource: "deployments", Verb: "get", Namespace: "team-a"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected permissions:\nwant: %#v\ngot:  %#v", want, got)
	}

	_, err = requiredToolPermissions(context.Background(), cs, "get_resource", map[string]interface{}{
		"kind":      "Pod",
		"name":      "nginx",
		"namespace": "team-b",
	})
	if err == nil {
		t.Fatalf("expected cross-namespace request to fail")
	}

	_, err = requiredToolPermissions(context.Background(), cs, "list_resources", map[string]interface{}{
		"kind": "Node",
	})
	if err == nil {
		t.Fatalf("expected cluster-scoped resource request to fail")
	}

	got, err = requiredToolPermissions(context.Background(), cs, "get_cluster_overview", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = []toolPermission{
		{Resource: "pods", Verb: "get", Namespace: "team-a"},
		{Resource: "services", Verb: "get", Namespace: "team-a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected overview permissions:\nwant: %#v\ngot:  %#v", want, got)
	}

	if _, err = requiredToolPermissions(context.Background(), cs, "query_prometheus", map[string]interface{}{"query": "up"}); err == nil {
		t.Fatalf("expected prometheus tool to be unavailable in namespace-scoped workspace")
	}

	got, err = requiredToolPermissions(context.Background(), cs, "list_helm_releases", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = []toolPermission{{Resource: "helmreleases", Verb: "get", Namespace: "team-a"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected helm list permissions:\nwant: %#v\ngot:  %#v", want, got)
	}

	_, err = requiredToolPermissions(context.Background(), cs, "get_helm_release", map[string]interface{}{
		"release_name": "api",
		"namespace":    "team-b",
	})
	if err == nil {
		t.Fatalf("expected cross-namespace helm request to fail")
	}
}
