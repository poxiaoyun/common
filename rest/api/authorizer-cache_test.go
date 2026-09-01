package api

import (
	"context"
	"testing"
	"time"

	"xiaoshiai.cn/common/authz"
)

func TestCacheAuthorizerKeysCompleteAuthorizationInput(t *testing.T) {
	calls := 0
	cached := NewCacheAuthorizer(authz.AuthorizerFunc(func(context.Context, Authentication, authz.Operation) (authz.EvaluationResult, error) {
		calls++
		return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
	}), 10, time.Minute)
	attributes := Attributes{Service: "cloud", Method: "GET", Action: "get", Path: "/instances/one"}
	inputs := []Authentication{
		{Subject: Subject{ID: "user", Groups: []string{"one"}}},
		{Subject: Subject{ID: "user", Groups: []string{"two"}}},
		{Subject: Subject{ID: "user"}, Actor: &Subject{ID: "worker-one"}},
		{Subject: Subject{ID: "user"}, Actor: &Subject{ID: "worker-two"}},
		{Subject: Subject{ID: "user"}, Token: &TokenInfo{Scopes: []string{"instances.read"}}},
		{Subject: Subject{ID: "user"}, Token: &TokenInfo{Scopes: []string{"instances.write"}}},
	}
	for _, authentication := range inputs {
		if _, err := cached.Authorize(t.Context(), authentication, authorizationOperation(attributes)); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(inputs) {
		t.Fatalf("authorizer calls = %d, want %d", calls, len(inputs))
	}
	if _, err := cached.Authorize(t.Context(), inputs[0], authorizationOperation(attributes)); err != nil {
		t.Fatal(err)
	}
	if calls != len(inputs) {
		t.Fatalf("cached input called authorizer again: %d", calls)
	}
}
