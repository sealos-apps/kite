package helmguard

import (
	"context"
	"strings"
	"testing"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	release "helm.sh/helm/v4/pkg/release/v1"
)

func TestAuthorizeResourceAllowsNamespacedResource(t *testing.T) {
	previous := rbac.RBACConfig
	defer func() { rbac.RBACConfig = previous }()
	rbac.RBACConfig = &common.RolesConfig{}

	user := model.User{
		Username: "alice",
		Roles: []common.Role{{
			Name:       "team-a-editor",
			Clusters:   []string{"cluster-a"},
			Namespaces: []string{"team-a"},
			Resources:  []string{"configmaps"},
			Verbs:      []string{"create", "update"},
		}},
	}
	cs := &cluster.ClientSet{Name: "cluster-a"}

	err := AuthorizeResource(context.Background(), user, cs, helmReleaseResource("ConfigMap", "team-a"), "team-a", string(common.VerbCreate))
	if err != nil {
		t.Fatalf("expected namespaced resource to be allowed, got %v", err)
	}
}

func TestAuthorizeResourceRejectsCrossNamespaceResource(t *testing.T) {
	user := model.User{
		Username: "alice",
		Roles: []common.Role{{
			Name:       "team-a-editor",
			Clusters:   []string{"cluster-a"},
			Namespaces: []string{"team-a"},
			Resources:  []string{"configmaps"},
			Verbs:      []string{"create", "update"},
		}},
	}
	cs := &cluster.ClientSet{Name: "cluster-a"}

	err := AuthorizeResource(context.Background(), user, cs, helmReleaseResource("ConfigMap", "team-b"), "team-a", string(common.VerbCreate))
	if err == nil || !strings.Contains(err.Error(), "targets namespace team-b") {
		t.Fatalf("expected cross-namespace rejection, got %v", err)
	}
}

func TestAuthorizeResourceRejectsClusterScopedForNonAdmin(t *testing.T) {
	user := model.User{
		Username: "alice",
		Roles: []common.Role{{
			Name:       "team-a-editor",
			Clusters:   []string{"cluster-a"},
			Namespaces: []string{"team-a"},
			Resources:  []string{"*"},
			Verbs:      []string{"*"},
		}},
	}
	cs := &cluster.ClientSet{Name: "cluster-a"}

	err := AuthorizeResource(context.Background(), user, cs, helmReleaseResource("ClusterRole", ""), "team-a", string(common.VerbCreate))
	if err == nil || !strings.Contains(err.Error(), "requires admin role") {
		t.Fatalf("expected cluster-scoped resource rejection, got %v", err)
	}
}

func TestAuthorizeReleaseChangeRequiresDeleteForRemovedResources(t *testing.T) {
	previous := rbac.RBACConfig
	defer func() { rbac.RBACConfig = previous }()
	rbac.RBACConfig = &common.RolesConfig{}

	user := model.User{
		Username: "alice",
		Roles: []common.Role{{
			Name:       "team-a-editor",
			Clusters:   []string{"cluster-a"},
			Namespaces: []string{"team-a"},
			Resources:  []string{"configmaps", "secrets"},
			Verbs:      []string{"create", "update"},
		}},
	}
	cs := &cluster.ClientSet{Name: "cluster-a"}
	current := manifestRelease("team-a", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kept
---
apiVersion: v1
kind: Secret
metadata:
  name: removed
`)
	next := manifestRelease("team-a", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kept
`)

	err := AuthorizeReleaseChange(context.Background(), user, cs, current, next)
	if err == nil || !strings.Contains(err.Error(), "delete secrets") {
		t.Fatalf("expected missing delete permission for removed Secret, got %v", err)
	}
}

func TestAuthorizeReleaseChangeAllowsAddedUpdatedAndRemovedResources(t *testing.T) {
	previous := rbac.RBACConfig
	defer func() { rbac.RBACConfig = previous }()
	rbac.RBACConfig = &common.RolesConfig{}

	user := model.User{
		Username: "alice",
		Roles: []common.Role{{
			Name:       "team-a-editor",
			Clusters:   []string{"cluster-a"},
			Namespaces: []string{"team-a"},
			Resources:  []string{"configmaps", "secrets"},
			Verbs:      []string{"create", "update", "delete"},
		}},
	}
	cs := &cluster.ClientSet{Name: "cluster-a"}
	current := manifestRelease("team-a", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kept
---
apiVersion: v1
kind: Secret
metadata:
  name: removed
`)
	next := manifestRelease("team-a", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kept
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: added
`)

	err := AuthorizeReleaseChange(context.Background(), user, cs, current, next)
	if err != nil {
		t.Fatalf("expected release change to be allowed, got %v", err)
	}
}

func helmReleaseResource(kind, namespace string) helmutil.HelmReleaseResource {
	return helmutil.HelmReleaseResource{
		APIVersion: "v1",
		Kind:       kind,
		Name:       strings.ToLower(kind) + "-example",
		Namespace:  namespace,
	}
}

func manifestRelease(namespace, manifest string) *release.Release {
	return &release.Release{
		Namespace: namespace,
		Manifest:  manifest,
	}
}
