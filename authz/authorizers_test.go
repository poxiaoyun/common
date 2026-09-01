package authz_test

import (
	"context"
	"testing"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/authz"
)

func TestAuthorizerChainContinuesOnlyForNoOpinion(t *testing.T) {
	called := false
	chain := authz.AuthorizerChain{
		authz.AuthorizerFunc(func(
			context.Context,
			authn.Authentication,
			authz.Operation,
		) (authz.EvaluationResult, error) {
			return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, nil
		}),
		authz.AuthorizerFunc(func(
			context.Context,
			authn.Authentication,
			authz.Operation,
		) (authz.EvaluationResult, error) {
			called = true
			return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
		}),
	}

	result, err := chain.Authorize(t.Context(), authn.Authentication{}, authz.Operation{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.DecisionAllow || !called {
		t.Fatalf("Authorize() decision = %q, later authorizer called = %v", result.Decision, called)
	}
}

func TestGroupAuthorizerUsesAuthenticatedSubjectGroups(t *testing.T) {
	authorizer := authz.NewGroupAuthorizer([]string{"operators"}, []string{"banned"})
	authentication := authn.Authentication{
		Subject: authn.Subject{Groups: []string{"banned", "operators"}},
	}

	result, err := authorizer.Authorize(t.Context(), authentication, authz.Operation{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.DecisionDeny {
		t.Fatalf("Authorize() decision = %q, want %q", result.Decision, authz.DecisionDeny)
	}
}
