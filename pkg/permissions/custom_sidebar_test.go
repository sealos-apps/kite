package permissions

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestCanCreateCustomCRDGroup(t *testing.T) {
	tests := []struct {
		name        string
		user        model.User
		clusterName string
		want        bool
	}{
		{
			name: "admin role can create",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "admin",
						Clusters:   []string{"*"},
						Namespaces: []string{"*"},
						Resources:  []string{"*"},
						Verbs:      []string{"*"},
					},
				},
			},
			clusterName: "cluster-a",
			want:        true,
		},
		{
			name: "exempt-like full role on current cluster can create",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "sealos-role",
						Clusters:   []string{"sealos-tenant-a"},
						Namespaces: []string{"*"},
						Resources:  []string{"*"},
						Verbs:      []string{"*"},
					},
				},
			},
			clusterName: "sealos-tenant-a",
			want:        true,
		},
		{
			name: "namespace-scoped role cannot create",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "sealos-role",
						Clusters:   []string{"sealos-tenant-a"},
						Namespaces: []string{"tenant-a"},
						Resources:  []string{"*"},
						Verbs:      []string{"*"},
					},
				},
			},
			clusterName: "sealos-tenant-a",
			want:        false,
		},
		{
			name: "cluster mismatch cannot create",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "sealos-role",
						Clusters:   []string{"sealos-tenant-a"},
						Namespaces: []string{"*"},
						Resources:  []string{"*"},
						Verbs:      []string{"*"},
					},
				},
			},
			clusterName: "sealos-tenant-b",
			want:        false,
		},
		{
			name: "fallback role cluster works without clusterName",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "sealos-role",
						Clusters:   []string{"sealos-tenant-a"},
						Namespaces: []string{"*"},
						Resources:  []string{"*"},
						Verbs:      []string{"*"},
					},
				},
			},
			clusterName: "",
			want:        true,
		},
		{
			name: "limited resource permission cannot create",
			user: model.User{
				Roles: []common.Role{
					{
						Name:       "custom",
						Clusters:   []string{"*"},
						Namespaces: []string{"*"},
						Resources:  []string{"pods"},
						Verbs:      []string{"create"},
					},
				},
			},
			clusterName: "cluster-a",
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanCreateCustomCRDGroup(tc.user, tc.clusterName)
			if got != tc.want {
				t.Fatalf("CanCreateCustomCRDGroup() = %v, want %v", got, tc.want)
			}
		})
	}
}
