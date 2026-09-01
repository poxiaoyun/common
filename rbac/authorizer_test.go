package rbac_test

import (
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/rbac"
	"xiaoshiai.cn/common/store"
)

func TestScopedPermissionMatch(t *testing.T) {
	tests := []struct {
		name        string
		scopes      []store.Scope
		permissions []authz.Permission
		operation   authz.Operation
		want        bool
	}{
		{
			permissions: []authz.Permission{{Service: "*", Actions: []string{"get", "list"}, Resources: []string{"**"}}},
			operation: authz.Operation{
				Action:   "get",
				Resource: resourcePath(authz.ResourceReference{Type: "namespaces", ID: "default"}),
			},
			want: true,
		},
		{
			permissions: []authz.Permission{{Service: "*", Actions: []string{"get", "list"}, Resources: []string{"applications:**"}}},
			operation: authz.Operation{
				Action:   "list",
				Resource: resourcePath(authz.ResourceReference{Type: "applications"}),
			},
			want: true,
		},
		{
			permissions: []authz.Permission{{Service: "*", Actions: []string{"*"}, Resources: []string{"applications:**"}}},
			operation: authz.Operation{
				Action:   "get",
				Resource: resourcePath(authz.ResourceReference{Type: "applications", ID: "default"}),
			},
			want: true,
		},
		{
			permissions: []authz.Permission{
				{Service: "*", Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			operation: authz.Operation{
				Action: "get",
				Resource: resourcePath(
					authz.ResourceReference{Type: "applications", ID: "default"},
					authz.ResourceReference{Type: "resources", ID: "pods"},
				),
			},
			want: true,
		},
		{
			permissions: []authz.Permission{{Service: "*", Actions: []string{"*"}, Resources: []string{"applications:**"}}},
			operation: authz.Operation{
				Action:   "list",
				Resource: resourcePath(authz.ResourceReference{Type: "applications"}),
			},
			want: true,
		},
		{
			scopes: []store.Scope{
				{Resource: "tenants", Name: "default"},
			},
			permissions: []authz.Permission{
				{Service: "*", Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			operation: authz.Operation{
				Action: "list",
				Resource: resourcePath(
					authz.ResourceReference{Type: "tenants", ID: "default"},
					authz.ResourceReference{Type: "applications"},
				),
			},
			want: true,
		},
		{
			scopes: []store.Scope{
				{Resource: "tenants", Name: "abc"},
			},
			permissions: []authz.Permission{
				{Service: "*", Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			operation: authz.Operation{
				Action: "list",
				Resource: resourcePath(
					authz.ResourceReference{Type: "tenants", ID: "default"},
					authz.ResourceReference{Type: "applications"},
				),
			},
			want: false,
		},
		{
			name: "service mismatch",
			permissions: []authz.Permission{
				{Service: "cloud", Actions: []string{"list"}, Resources: []string{"applications"}},
			},
			operation: authz.Operation{Service: "apps", Action: "list", Resource: resourcePath(authz.ResourceReference{Type: "applications"})},
			want:      false,
		},
		{
			name: "later permission matches",
			permissions: []authz.Permission{
				{Service: "apps", Actions: []string{"get"}, Resources: []string{"applications"}},
				{Service: "apps", Actions: []string{"list"}, Resources: []string{"applications"}},
			},
			operation: authz.Operation{Service: "apps", Action: "list", Resource: resourcePath(authz.ResourceReference{Type: "applications"})},
			want:      true,
		},
		{
			name:   "binding scope itself requires permission",
			scopes: []store.Scope{{Resource: "tenants", Name: "default"}},
			operation: authz.Operation{
				Service:  "apps",
				Action:   "get",
				Resource: resourcePath(authz.ResourceReference{Type: "tenants", ID: "default"}),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rbac.ScopedPermissionMatch(tt.scopes, tt.permissions, tt.operation); got != tt.want {
				t.Errorf("ScopedPermissionMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func resourcePath(references ...authz.ResourceReference) authz.Resource {
	target := references[len(references)-1]
	return authz.Resource{Type: target.Type, ID: target.ID, Scope: references[:len(references)-1]}
}
