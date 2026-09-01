package authz_test

import (
	"encoding/json"
	"testing"

	"xiaoshiai.cn/common/authz"
)

func TestOperationJSONUsesCanonicalFields(t *testing.T) {
	operation := authz.Operation{
		Service: "apps",
		Action:  "get",
		Resource: authz.Resource{
			Type:       "apps.application",
			ID:         "console",
			Scope:      authz.Scope{{Type: "iam.organization", ID: "acme"}},
			Properties: authz.Properties{"visibility": "public"},
		},
		Context: authz.Context{"http.method": "GET"},
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"service":"apps","action":"get","resource":{"type":"apps.application","id":"console","scope":[{"type":"iam.organization","id":"acme"}],"properties":{"visibility":"public"}},"context":{"http.method":"GET"}}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestEvaluationResultJSONUsesCanonicalFields(t *testing.T) {
	result := authz.EvaluationResult{
		Decision: authz.DecisionAllow,
		Reason:   "policy matched",
		Snapshot: "authorization-state",
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"decision":"Allow","reason":"policy matched","snapshot":"authorization-state"}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestScopeJSONUsesCanonicalReferenceFields(t *testing.T) {
	scope := authz.Scope{{Type: "iam.organization", ID: "organization-1"}}
	encoded, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `[{"type":"iam.organization","id":"organization-1"}]` {
		t.Fatalf("JSON = %s", encoded)
	}

	root, err := json.Marshal(authz.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if string(root) != `[]` {
		t.Fatalf("root JSON = %s", root)
	}
}

func TestWithAtLeastAppliesToCheckAndBatchCheck(t *testing.T) {
	snapshot := "authorization-state"

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
