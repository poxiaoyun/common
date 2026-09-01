package authz_test

import (
	"encoding/json"
	"testing"

	"xiaoshiai.cn/common/authz"
)

func TestPermissionJSONUsesCanonicalFields(t *testing.T) {
	encoded, err := json.Marshal(authz.Permission{Service: "moha", Actions: []string{"get"}, Resources: []string{"repositories:*"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"service":"moha","actions":["get"],"resources":["repositories:*"]}` {
		t.Fatalf("JSON = %s", encoded)
	}
}

func TestMatchPermission(t *testing.T) {
	base := authz.Operation{
		Service: "apps",
		Action:  "get",
		Resource: authz.Resource{
			Type:  "apps.application",
			ID:    "console",
			Scope: authz.Scope{{Type: "iam.organization", ID: "acme"}},
		},
	}
	tests := []struct {
		name       string
		permission authz.Permission
		operation  authz.Operation
		want       bool
	}{
		{
			name:       "matching service action and nested resource",
			permission: authz.Permission{Service: "apps", Actions: []string{"get"}, Resources: []string{"iam.organization:*:apps.application:**"}},
			operation:  base,
			want:       true,
		},
		{
			name:       "different service",
			permission: authz.Permission{Service: "cloud", Actions: []string{"get"}, Resources: []string{"iam.organization:*:apps.application:**"}},
			operation:  base,
		},
		{
			name:       "permission action wildcard",
			permission: authz.Permission{Service: "apps", Actions: []string{"*"}, Resources: []string{"iam.organization:*:apps.application:**"}},
			operation:  base,
			want:       true,
		},
		{
			name:       "input action is not a wildcard",
			permission: authz.Permission{Service: "apps", Actions: []string{"list"}, Resources: []string{"iam.organization:*:apps.application:**"}},
			operation: authz.Operation{
				Service:  "apps",
				Action:   "*",
				Resource: base.Resource,
			},
		},
		{
			name:       "collection omits terminal ID",
			permission: authz.Permission{Service: "apps", Actions: []string{"list"}, Resources: []string{"iam.organization:*:apps.application"}},
			operation: authz.Operation{
				Service: "apps",
				Action:  "list",
				Resource: authz.Resource{
					Type:  "apps.application",
					Scope: authz.Scope{{Type: "iam.organization", ID: "acme"}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authz.MatchPermission(tt.permission, tt.operation); got != tt.want {
				t.Fatalf("MatchPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}
