package rbac_test

import (
	"encoding/json"
	"testing"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/rbac"
)

func TestScopedUserRoleJSONUsesSubjectID(t *testing.T) {
	binding := rbac.ScopedUserRole{
		User:     "subject-1",
		UserInfo: &authn.Subject{ID: "subject-1", Name: "alice"},
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		User     string         `json:"user"`
		UserInfo *authn.Subject `json:"userInfo"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.User != "subject-1" || wire.UserInfo == nil || wire.UserInfo.ID != wire.User {
		t.Fatalf("ScopedUserRole wire value = %#v", wire)
	}
}
