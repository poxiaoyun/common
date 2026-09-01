package authz_test

import (
	"encoding/json"
	"testing"

	"xiaoshiai.cn/common/authz"
)

func TestScopeConstructorsKeepRootAndResourcePathsDistinct(t *testing.T) {
	root := authz.Scope{}
	if len(root.Path) != 0 {
		t.Fatalf("root Scope = %#v", root)
	}

	path := []authz.ResourceReference{
		{Type: "iam.organization", ID: "organization-1"},
		{Type: "moha.repository", ID: "repository-1"},
	}
	scope := authz.ResourceScope(path...)
	path[0].ID = "changed"
	if scope.Path[0].ID != "organization-1" {
		t.Fatalf("ResourceScope() = %#v", scope)
	}
}

func TestScopeJSONUsesCanonicalPathAndReferenceFields(t *testing.T) {
	scope := authz.ResourceScope(authz.ResourceReference{Type: "iam.organization", ID: "organization-1"})
	encoded, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"path":[{"type":"iam.organization","id":"organization-1"}]}` {
		t.Fatalf("JSON = %s", encoded)
	}

	root, err := json.Marshal(authz.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if string(root) != `{}` {
		t.Fatalf("root JSON = %s", root)
	}
}
