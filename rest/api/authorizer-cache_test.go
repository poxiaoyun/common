package api

import (
	"context"
	"testing"
	"time"
)

func TestCacheAuthorizerKeysCompleteAuthorizationInput(t *testing.T) {
	calls := 0
	cached := NewCacheAuthorizer(AuthorizerFunc(func(context.Context, AuthenticationInfo, Attributes) (Decision, string, error) {
		calls++
		return DecisionAllow, "", nil
	}), 10, time.Minute)
	attributes := Attributes{Service: "cloud", Method: "GET", Action: "get", Path: "/instances/one"}
	inputs := []AuthenticationInfo{
		{Subject: Subject{ID: "user", Groups: []string{"one"}}},
		{Subject: Subject{ID: "user", Groups: []string{"two"}}},
		{Subject: Subject{ID: "user"}, Actor: &Subject{ID: "worker-one"}},
		{Subject: Subject{ID: "user"}, Actor: &Subject{ID: "worker-two"}},
		{Subject: Subject{ID: "user"}, Access: &AccessConstraints{Scopes: []string{"instances.read"}}},
		{Subject: Subject{ID: "user"}, Access: &AccessConstraints{Scopes: []string{"instances.write"}}},
	}
	for _, authentication := range inputs {
		if _, _, err := cached.Authorize(t.Context(), authentication, attributes); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(inputs) {
		t.Fatalf("authorizer calls = %d, want %d", calls, len(inputs))
	}
	if _, _, err := cached.Authorize(t.Context(), inputs[0], attributes); err != nil {
		t.Fatal(err)
	}
	if calls != len(inputs) {
		t.Fatalf("cached input called authorizer again: %d", calls)
	}
}
