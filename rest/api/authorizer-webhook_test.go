package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

func TestWebhookAuthorizerSendsCompleteAuthentication(t *testing.T) {
	authentication := api.Authentication{
		Subject: api.Subject{Type: "iam.user", ID: "user", Groups: []string{"developers"}},
		Actor:   &api.Subject{Type: "iam.workload", ID: "worker"},
		Token:   &api.TokenInfo{Scopes: []string{"instances.read"}},
	}
	attributes := api.Attributes{Service: "cloud", Action: "get", Path: "/instances/one"}
	request := &api.AuthorizationReview{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(api.AuthorizationReview{Status: &authz.EvaluationResult{Decision: authz.DecisionAllow}})
	}))
	defer server.Close()
	authorizer, err := api.NewWebhookAuthorizer(&api.WebhookAuthorizerOptions{Options: httpclient.Options{Server: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	filter := api.NewAuthorizationFilter(authorizer)
	httpRequest := httptest.NewRequest(http.MethodGet, attributes.Path, nil)
	httpRequest = httpRequest.WithContext(api.WithAuthentication(httpRequest.Context(), authentication))
	httpRequest = httpRequest.WithContext(api.WithAttributes(httpRequest.Context(), &attributes))
	response := httptest.NewRecorder()
	filter.Process(response, httpRequest, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	if response.Code != http.StatusOK || request.Spec == nil ||
		!reflect.DeepEqual(request.Spec.Authentication, authentication) ||
		request.Spec.Operation.Service != "cloud" || request.Spec.Operation.Action != "get" || request.Spec.Operation.Path != attributes.Path {
		t.Fatalf("request = %#v, status = %d", request, response.Code)
	}
}

func TestAuthorizationClientPreservesFactsAndBatchOrder(t *testing.T) {
	authentication := api.Authentication{
		Subject: api.Subject{Type: "human", ID: "user-1", Groups: []string{"developers"}},
		Actor:   &api.Subject{Type: "human", ID: "operator-1"},
		Token:   &api.TokenInfo{Audiences: []string{"apps"}, Scopes: []string{"read:applications"}},
	}
	operation := authz.Operation{
		Service: "apps",
		Action:  "read",
		Resource: authz.Resource{
			Type:       "applications",
			ID:         "one",
			Scope:      authz.Scope{{Type: "organizations", ID: "acme"}},
			Properties: authz.Properties{"revision": int64(9007199254740993), "visibility": "public"},
		},
		Context: authz.Context{"request.ip": "192.0.2.1"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer service-credential" {
			http.Error(w, "untrusted assertion source", http.StatusUnauthorized)
			return
		}
		encoder := json.NewEncoder(w)
		decoder := json.NewDecoder(r.Body)
		switch r.URL.Path {
		case "/v1/authorization-reviews":
			var request api.AuthorizationReview
			if err := decoder.Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Spec == nil || !reflect.DeepEqual(request.Spec.Authentication, authentication) || !reflect.DeepEqual(request.Spec.Operation, operation) {
				t.Errorf("review lost authoritative input: %#v", request.Spec)
				http.Error(w, "invalid facts", http.StatusBadRequest)
				return
			}
			_ = encoder.Encode(api.AuthorizationReview{Status: &authz.EvaluationResult{Decision: authz.DecisionNoOpinion, Snapshot: "state-2"}})
		case "/v1/authorization-checks":
			var request api.AuthorizationCheck
			if err := decoder.Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Spec == nil || !reflect.DeepEqual(request.Spec.Authentication, authentication) || !reflect.DeepEqual(request.Spec.Operation, operation) || request.Spec.AtLeast != "state-1" {
				t.Errorf("check lost authoritative input: %#v", request.Spec)
				http.Error(w, "invalid facts", http.StatusBadRequest)
				return
			}
			_ = encoder.Encode(api.AuthorizationCheck{Status: &authz.EvaluationResult{Decision: authz.DecisionAllow, Snapshot: "state-2"}})
		case "/v1/authorization-batch-checks":
			var request api.AuthorizationBatchCheck
			if err := decoder.Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Spec == nil || !reflect.DeepEqual(request.Spec.Authentication, authentication) || len(request.Spec.Operations) != 2 || request.Spec.AtLeast != "state-1" {
				t.Errorf("invalid batch input: %#v", request.Spec)
				return
			}
			decisions := make([]authz.CheckDecision, len(request.Spec.Operations))
			for index, operation := range request.Spec.Operations {
				decision := authz.DecisionDeny
				if operation.Resource.Properties["visibility"] == "public" {
					decision = authz.DecisionAllow
				}
				decisions[index] = authz.CheckDecision{Decision: decision, Reason: operation.Resource.ID}
			}
			_ = encoder.Encode(api.AuthorizationBatchCheck{Status: &authz.BatchCheckResult{Decisions: decisions, Snapshot: "state-2"}})
		case "/v1/authorization-constraint-plans":
			var request api.AuthorizationConstraintPlan
			if err := decoder.Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Spec == nil || !reflect.DeepEqual(request.Spec.Authentication, authentication) || request.Spec.Operation.Resource.ID != "" || request.Spec.AtLeast != "state-1" {
				t.Errorf("invalid plan input: %#v", request.Spec)
				return
			}
			_ = encoder.Encode(api.AuthorizationConstraintPlan{Status: &authz.ResourceConstraintPlan{
				Constraint: authz.ResourceConstraint{
					Operator: authz.ConstraintProperties,
					Properties: authz.ResourcePropertyConstraint{
						Expression: authz.GreaterThan(authz.ResourceProperty("apps", "revision"), authz.Literal(int64(9007199254740992))),
						Result:     true,
					},
				},
				Snapshot: "state-2",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	transport, err := httpclient.NewClientFromOptions(&httpclient.Options{Server: server.URL + "/v1", Token: "service-credential"})
	if err != nil {
		t.Fatal(err)
	}
	client := api.AuthorizationClient{Client: transport}
	gate, err := client.Authorize(t.Context(), authentication, operation)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != authz.DecisionNoOpinion || gate.Snapshot != "state-2" {
		t.Fatalf("gate result = %#v", gate)
	}
	check, err := client.Check(t.Context(), authentication, operation, authz.WithAtLeast("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	if check.Decision != authz.DecisionAllow || check.Snapshot != "state-2" {
		t.Fatalf("check result = %#v", check)
	}
	private := operation
	private.Resource.ID = "two"
	private.Resource.Properties = authz.Properties{"visibility": "private"}
	batch, err := client.BatchCheck(t.Context(), authentication, []authz.Operation{private, operation}, authz.WithAtLeast("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	if batch.Snapshot != "state-2" || !reflect.DeepEqual(batch.Decisions, []authz.CheckDecision{{Decision: authz.DecisionDeny, Reason: "two"}, {Decision: authz.DecisionAllow, Reason: "one"}}) {
		t.Fatalf("batch result = %#v", batch)
	}
	collection := operation
	collection.Resource.ID = ""
	collection.Resource.Properties = nil
	plan, err := client.PlanResourceConstraint(t.Context(), authentication, collection, authz.WithAtLeast("state-1"))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Snapshot != "state-2" || !plan.Constraint.Properties.Match(operation.Resource) || plan.Constraint.Properties.Match(private.Resource) {
		t.Fatalf("plan changed candidate selection: %#v", plan)
	}
}

func TestAuthorizationClientRejectsMalformedResponses(t *testing.T) {
	for _, test := range []struct {
		method string
		body   string
	}{
		{"review", `{}`},
		{"review", `{"status":{"decision":"unexpected"}}`},
		{"check", `{"status":null}`},
		{"check", `{"status":{"decision":"NoOpinion"}}`},
		{"check", `{"status":{"decision":"Allow"},`},
		{"check", `{"status":{"decision":"Allow"}} trailing`},
		{"check", `{"status":{"decision":"Allow"}} {"status":{"decision":"Deny"}}`},
		{"check", `{"status":{"decision":"Allow","unexpected":true}}`},
		{"batch", `{"status":null}`},
		{"batch", `{"status":{"decisions":[{"decision":"Allow"}]}}`},
		{"batch", `{"status":{"decisions":[{"decision":"Allow"},{"decision":"NoOpinion"}]}}`},
		{"plan", `{"status":null}`},
		{"plan", `{"status":{}}`},
		{"plan", `{"status":{"constraint":null}}`},
		{"plan", `{"status":{"constraint":{"operator":"not","constraints":[null]}}}`},
		{"plan", `{"status":{"constraint":{"operator":"not"}}}`},
		{"plan", `{"status":{"constraint":{"operator":"unknown"}}}`},
		{"plan", `{"status":{"constraint":{"operator":"all","deny":true}}}`},
		{"plan", `{"status":{"constraint":{"operator":"properties","properties":{"expression":{"operator":"equal","values":[{"source":"property","property":{"service":"apps","namespace":"resource","name":"revision"}},{"source":"literal","literal":{"type":"int64","value":9007199254740993}}]},"result":true}}}}`},
	} {
		t.Run(test.method+test.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			transport, err := httpclient.NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			result, err := callAuthorizationClient(t.Context(), api.AuthorizationClient{Client: transport}, test.method)
			if err != nil {
				value := reflect.ValueOf(result)
				if !value.IsZero() {
					t.Fatalf("failed call returned usable result: %#v", result)
				}
				return
			}
			t.Fatalf("malformed response produced usable result: %#v", result)
		})
	}
}

func TestAuthorizationClientPreservesStructuredErrors(t *testing.T) {
	for _, method := range []string{"review", "check", "batch", "plan"} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := errors.NewUnsupported("snapshot lower bound is unavailable")
				w.WriteHeader(int(status.Code))
				encoder := json.NewEncoder(w)
				_ = encoder.Encode(status)
			}))
			defer server.Close()
			transport, err := httpclient.NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			result, err := callAuthorizationClient(t.Context(), api.AuthorizationClient{Client: transport}, method)
			value := reflect.ValueOf(result)
			if !errors.IsUnsupported(err) || !value.IsZero() {
				t.Fatalf("structured error lost: result=%#v error=%v", result, err)
			}
		})
	}
}

func callAuthorizationClient(ctx context.Context, client api.AuthorizationClient, method string) (any, error) {
	operation := authz.Operation{Service: "apps", Action: "read", Resource: authz.Resource{Type: "applications", ID: "one"}}
	switch method {
	case "review":
		return client.Authorize(ctx, api.Authentication{}, operation)
	case "check":
		return client.Check(ctx, api.Authentication{}, operation)
	case "batch":
		return client.BatchCheck(ctx, api.Authentication{}, []authz.Operation{operation, operation})
	case "plan":
		operation.Resource.ID = ""
		return client.PlanResourceConstraint(ctx, api.Authentication{}, operation)
	default:
		panic("unknown authorization test method")
	}
}
