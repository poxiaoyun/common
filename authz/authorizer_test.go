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
		authz.AuthorizerFunc(func(context.Context, authz.AuthorizeInput) (authz.AuthorizeResult, error) {
			return authz.AuthorizeResult{Decision: authz.AuthorizeNoOpinion}, nil
		}),
		authz.AuthorizerFunc(func(context.Context, authz.AuthorizeInput) (authz.AuthorizeResult, error) {
			called = true
			return authz.AuthorizeResult{Decision: authz.AuthorizeAllow}, nil
		}),
	}

	result, err := chain.Authorize(t.Context(), authz.AuthorizeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.AuthorizeAllow || !called {
		t.Fatalf("Authorize() = %#v, later authorizer called = %v", result, called)
	}
}

func TestGroupAuthorizerUsesAuthenticatedSubjectGroups(t *testing.T) {
	authorizer := authz.NewGroupAuthorizer([]string{"operators"}, []string{"banned"})
	input := authz.AuthorizeInput{Authentication: authn.Authentication{
		Subject: authn.Subject{Groups: []string{"banned", "operators"}},
	}}

	result, err := authorizer.Authorize(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.AuthorizeDeny {
		t.Fatalf("Authorize() decision = %q, want %q", result.Decision, authz.AuthorizeDeny)
	}
}

func TestMatchPermission(t *testing.T) {
	base := authz.AuthorizeInput{
		Service: "apps",
		Action:  "get",
		Resources: []authz.ResourceSegment{
			{Resource: "organizations", Name: "acme"},
			{Resource: "applications", Name: "console"},
		},
	}
	tests := []struct {
		name       string
		permission authz.Permission
		input      authz.AuthorizeInput
		want       bool
	}{
		{
			name:       "matching service action and nested resource",
			permission: authz.Permission{Service: "apps", Actions: []string{"get"}, Resources: []string{"organizations:*:applications:**"}},
			input:      base,
			want:       true,
		},
		{
			name:       "different service",
			permission: authz.Permission{Service: "cloud", Actions: []string{"get"}, Resources: []string{"organizations:*:applications:**"}},
			input:      base,
		},
		{
			name:       "permission action wildcard",
			permission: authz.Permission{Service: "apps", Actions: []string{"*"}, Resources: []string{"organizations:*:applications:**"}},
			input:      base,
			want:       true,
		},
		{
			name:       "input action is not a wildcard",
			permission: authz.Permission{Service: "apps", Actions: []string{"list"}, Resources: []string{"organizations:*:applications:**"}},
			input: authz.AuthorizeInput{
				Service:   "apps",
				Action:    "*",
				Resources: base.Resources,
			},
		},
		{
			name:       "collection omits terminal name",
			permission: authz.Permission{Service: "apps", Actions: []string{"list"}, Resources: []string{"organizations:*:applications"}},
			input: authz.AuthorizeInput{
				Service: "apps",
				Action:  "list",
				Resources: []authz.ResourceSegment{
					{Resource: "organizations", Name: "acme"},
					{Resource: "applications"},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authz.MatchPermission(tt.permission, tt.input); got != tt.want {
				t.Fatalf("MatchPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}
