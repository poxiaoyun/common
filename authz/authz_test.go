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

func TestWithAtLeastAppliesToCheckAndBatchCheck(t *testing.T) {
	snapshot := authz.AccessSnapshot("authorization-state")

	check := authz.ApplyCheckOptions(authz.WithAtLeast(snapshot))
	if check.AtLeast != snapshot {
		t.Fatalf("Check AtLeast = %q, want %q", check.AtLeast, snapshot)
	}

	batch := authz.ApplyBatchCheckOptions(authz.WithAtLeast(snapshot))
	if batch.AtLeast != snapshot {
		t.Fatalf("BatchCheck AtLeast = %q, want %q", batch.AtLeast, snapshot)
	}

	plan := authz.ApplyPlanResourceConstraintOptions(authz.WithAtLeast(snapshot))
	if plan.AtLeast != snapshot {
		t.Fatalf("PlanResourceConstraint AtLeast = %q, want %q", plan.AtLeast, snapshot)
	}
}
