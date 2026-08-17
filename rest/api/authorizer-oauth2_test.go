package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizationFilterUsesOAuth2ScopeAuthorizerInChain(t *testing.T) {
	tests := []struct {
		name              string
		authentication    AuthenticationInfo
		action            string
		fallbackDecision  Decision
		wantStatus        int
		wantChallenge     string
		wantFallbackCalls int
	}{
		{
			name: "scope authorizes access token",
			authentication: AuthenticationInfo{
				Subject: Subject{ID: "client"},
				Access:  &AccessConstraints{Scopes: []string{"orders.read"}},
			},
			action:            "get",
			fallbackDecision:  DecisionDeny,
			wantStatus:        http.StatusNoContent,
			wantFallbackCalls: 0,
		},
		{
			name: "missing scope denies access token",
			authentication: AuthenticationInfo{
				Subject: Subject{ID: "client"},
				Access:  &AccessConstraints{Scopes: []string{"orders.write"}},
			},
			action:            "get",
			fallbackDecision:  DecisionAllow,
			wantStatus:        http.StatusForbidden,
			wantChallenge:     `Bearer error="insufficient_scope"`,
			wantFallbackCalls: 0,
		},
		{
			name: "unmapped operation denies access token",
			authentication: AuthenticationInfo{
				Subject: Subject{ID: "client"},
				Access:  &AccessConstraints{Scopes: []string{"orders.read"}},
			},
			action:            "delete",
			fallbackDecision:  DecisionAllow,
			wantStatus:        http.StatusForbidden,
			wantChallenge:     `Bearer error="insufficient_scope"`,
			wantFallbackCalls: 0,
		},
		{
			name:              "non access-token authentication uses fallback policy",
			authentication:    AuthenticationInfo{Subject: Subject{ID: "user"}},
			action:            "get",
			fallbackDecision:  DecisionAllow,
			wantStatus:        http.StatusNoContent,
			wantFallbackCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackCalls := 0
			authorizer := AuthorizerChain{
				OAuth2ScopeAuthorizer{ResolveRequiredScope: func(attributes Attributes) string {
					if attributes.Action == "get" {
						return "orders.read"
					}
					return ""
				}},
				AuthorizerFunc(func(context.Context, AuthenticationInfo, Attributes) (Decision, string, error) {
					fallbackCalls++
					return tt.fallbackDecision, "", nil
				}),
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/orders", nil)
			request = request.WithContext(WithAuthentication(request.Context(), tt.authentication))
			request = request.WithContext(WithAttributes(request.Context(), &Attributes{Action: tt.action}))

			filter := NewAuthorizationFilter(authorizer)
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
	request = request.WithContext(WithAuthentication(request.Context(), AuthenticationInfo{
		Subject: Subject{ID: "client"},
		Access:  &AccessConstraints{Scopes: []string{"orders.read"}},
	}))
	request = request.WithContext(WithAttributes(request.Context(), &Attributes{Action: "get"}))

	filter := NewAuthorizationFilter(AuthorizerFunc(func(context.Context, AuthenticationInfo, Attributes) (Decision, string, error) {
		return DecisionDeny, "denied by resource policy", nil
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
