package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/rest/api"
)

func TestAuthorizationFilterUsesOAuth2ScopeAuthorizerInChain(t *testing.T) {
	tests := []struct {
		name              string
		authentication    api.Authentication
		action            string
		fallbackDecision  authz.Decision
		wantStatus        int
		wantChallenge     string
		wantFallbackCalls int
	}{
		{
			name: "scope authorizes access token",
			authentication: api.Authentication{
				Subject: api.Subject{ID: "client"},
				Token:   &api.TokenInfo{Scopes: []string{"orders.read"}},
			},
			action:            "get",
			fallbackDecision:  authz.DecisionDeny,
			wantStatus:        http.StatusNoContent,
			wantFallbackCalls: 0,
		},
		{
			name: "missing scope denies access token",
			authentication: api.Authentication{
				Subject: api.Subject{ID: "client"},
				Token:   &api.TokenInfo{Scopes: []string{"orders.write"}},
			},
			action:            "get",
			fallbackDecision:  authz.DecisionAllow,
			wantStatus:        http.StatusForbidden,
			wantChallenge:     `Bearer error="insufficient_scope"`,
			wantFallbackCalls: 0,
		},
		{
			name: "unmapped operation denies access token",
			authentication: api.Authentication{
				Subject: api.Subject{ID: "client"},
				Token:   &api.TokenInfo{Scopes: []string{"orders.read"}},
			},
			action:            "delete",
			fallbackDecision:  authz.DecisionAllow,
			wantStatus:        http.StatusForbidden,
			wantChallenge:     `Bearer error="insufficient_scope"`,
			wantFallbackCalls: 0,
		},
		{
			name:              "non access-token authentication uses fallback policy",
			authentication:    api.Authentication{Subject: api.Subject{ID: "user"}},
			action:            "get",
			fallbackDecision:  authz.DecisionAllow,
			wantStatus:        http.StatusNoContent,
			wantFallbackCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackCalls := 0
			authorizer := authz.AuthorizerChain{
				api.OAuth2ScopeAuthorizer{MatchScope: func(scope string, operation authz.Operation) bool {
					return scope == "orders.read" && operation.Action == "get"
				}},
				authz.AuthorizerFunc(func(context.Context, api.Authentication, authz.Operation) (authz.EvaluationResult, error) {
					fallbackCalls++
					return authz.EvaluationResult{Decision: tt.fallbackDecision}, nil
				}),
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/orders", nil)
			request = request.WithContext(api.WithAuthentication(request.Context(), tt.authentication))
			request = request.WithContext(api.WithAttributes(request.Context(), &api.Attributes{Action: tt.action}))

			filter := api.NewAuthorizationFilter(authorizer)
			filter.Process(response, request, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if got := response.Header().Get("WWW-Authenticate"); got != tt.wantChallenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, tt.wantChallenge)
			}
			if fallbackCalls != tt.wantFallbackCalls {
				t.Fatalf("fallback authorizer calls = %d, want %d", fallbackCalls, tt.wantFallbackCalls)
			}
		})
	}
}

func TestAuthorizationFilterDoesNotReportBusinessDenialAsInsufficientScope(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/orders", nil)
	request = request.WithContext(api.WithAuthentication(request.Context(), api.Authentication{
		Subject: api.Subject{ID: "client"},
		Token:   &api.TokenInfo{Scopes: []string{"orders.read"}},
	}))
	request = request.WithContext(api.WithAttributes(request.Context(), &api.Attributes{Action: "get"}))

	filter := api.NewAuthorizationFilter(authz.AuthorizerFunc(func(context.Context, api.Authentication, authz.Operation) (authz.EvaluationResult, error) {
		return authz.EvaluationResult{Decision: authz.DecisionDeny, Reason: "denied by resource policy"}, nil
	}))
	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached handler")
	}))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if challenge := response.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Fatalf("WWW-Authenticate = %q, want empty", challenge)
	}
}

func TestDefaultOAuth2ScopeMatcher(t *testing.T) {
	type input struct {
		scope      string
		attributes api.Attributes
	}
	tests := []struct {
		name     string
		input    input
		expected bool
	}{
		{
			name: "exact action",
			input: input{
				scope:      "create:orders",
				attributes: api.Attributes{Action: "create", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "arbitrary exact action",
			input: input{
				scope:      "approve:orders",
				attributes: api.Attributes{Action: "approve", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "read covers get",
			input: input{
				scope:      "read:orders",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "read covers list",
			input: input{
				scope:      "read:orders",
				attributes: api.Attributes{Action: "list", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "write covers update",
			input: input{
				scope:      "write:orders",
				attributes: api.Attributes{Action: "update", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "write covers domain action",
			input: input{
				scope:      "write:orders",
				attributes: api.Attributes{Action: "approve", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: true,
		},
		{
			name: "dotted resource",
			input: input{
				scope:      "read:a.b",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "a.b"}}},
			},
			expected: true,
		},
		{
			name: "resource containing colon",
			input: input{
				scope:      "read:orders:archive",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "orders:archive"}}},
			},
			expected: true,
		},
		{
			name: "different resource",
			input: input{
				scope:      "read:orders",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "users"}}},
			},
			expected: false,
		},
		{
			name: "read does not cover write",
			input: input{
				scope:      "read:orders",
				attributes: api.Attributes{Action: "update", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
		{
			name: "exact action does not cover another action",
			input: input{
				scope:      "create:orders",
				attributes: api.Attributes{Action: "update", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
		{
			name: "custom scope has no default meaning",
			input: input{
				scope:      "orders.manage",
				attributes: api.Attributes{Action: "update", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
		{
			name: "parent resource does not cover nested target",
			input: input{
				scope: "read:clusters",
				attributes: api.Attributes{Action: "list", Resources: []api.AttributeResource{
					{Resource: "clusters", Name: "cluster-a"},
					{Resource: "flavors"},
				}},
			},
			expected: false,
		},
		{
			name: "scope without separator",
			input: input{
				scope:      "read-orders",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
		{
			name: "scope without action",
			input: input{
				scope:      ":orders",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
		{
			name: "scope without resource",
			input: input{
				scope:      "read:",
				attributes: api.Attributes{Action: "get", Resources: []api.AttributeResource{{Resource: "orders"}}},
			},
			expected: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := api.DefaultOAuth2ScopeMatcher(test.input.scope, oauthOperation(test.input.attributes))
			if actual != test.expected {
				t.Fatalf("api.DefaultOAuth2ScopeMatcher(%q, %#v) = %t, want %t", test.input.scope, test.input.attributes, actual, test.expected)
			}
		})
	}
}

func TestNewOAuth2ScopeMatcher(t *testing.T) {
	matchResource := func(grantedResource string, operation authz.Operation) bool {
		return grantedResource == "catalog" && operation.Resource.Type == "flavors"
	}
	match := api.NewOAuth2ScopeMatcher(api.SafeMethodOAuth2ScopeActionMatcher, matchResource)

	for _, test := range []struct {
		name       string
		scope      string
		attributes api.Attributes
		expected   bool
	}{
		{
			name:       "safe method aggregate",
			scope:      "read:catalog",
			attributes: api.Attributes{Method: http.MethodGet, Action: "query", Resources: []api.AttributeResource{{Resource: "flavors"}}},
			expected:   true,
		},
		{
			name:       "write method aggregate",
			scope:      "write:catalog",
			attributes: api.Attributes{Method: http.MethodPost, Action: "publish", Resources: []api.AttributeResource{{Resource: "flavors"}}},
			expected:   true,
		},
		{
			name:       "arbitrary exact action precedes aggregate matcher",
			scope:      "query:catalog",
			attributes: api.Attributes{Method: http.MethodGet, Action: "query", Resources: []api.AttributeResource{{Resource: "flavors"}}},
			expected:   true,
		},
		{
			name:       "resource adapter rejects different logical resource",
			scope:      "read:flavors",
			attributes: api.Attributes{Method: http.MethodGet, Action: "list", Resources: []api.AttributeResource{{Resource: "flavors"}}},
			expected:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := match(test.scope, oauthOperation(test.attributes)); actual != test.expected {
				t.Fatalf("Match(%q, %#v) = %t, want %t", test.scope, test.attributes, actual, test.expected)
			}
		})
	}
}

func oauthOperation(attributes api.Attributes) authz.Operation {
	operation := authz.Operation{
		Service: attributes.Service,
		Action:  attributes.Action,
		Context: authz.Context{
			"http.method": attributes.Method,
			"http.path":   attributes.Path,
		},
	}
	if len(attributes.Resources) == 0 {
		operation.Path = attributes.Path
		return operation
	}
	operation.Resource.Scope = make(authz.Scope, len(attributes.Resources)-1)
	for index, resource := range attributes.Resources[:len(attributes.Resources)-1] {
		operation.Resource.Scope[index] = authz.ResourceReference{Type: resource.Resource, ID: resource.Name}
	}
	target := attributes.Resources[len(attributes.Resources)-1]
	operation.Resource.Type = target.Resource
	operation.Resource.ID = target.Name
	return operation
}
