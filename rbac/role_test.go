package rbac_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/rbac"
)

func TestRoleJSONUsesCanonicalPermissions(t *testing.T) {
	role := rbac.Role{Authorities: []authz.Permission{{
		Service: "apps", Actions: []string{"get"}, Resources: []string{"applications:*"},
	}}}
	encoded, err := json.Marshal(role)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Authorities []authz.Permission `json:"authorities"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire.Authorities, role.Authorities) {
		t.Fatalf("Role authorities = %#v, want %#v", wire.Authorities, role.Authorities)
	}
}
