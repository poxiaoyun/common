package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOAuth2RequiredScopeWithPrefix(t *testing.T) {
	required := OAuth2RequiredScope(func(attributes Attributes) string {
		if attributes.Action == "get" {
			return "read"
		}
		return ""
	}).WithPrefix("orders")

	if scope := required(Attributes{Action: "get"}); scope != "orders.read" {
		t.Fatalf("scope = %q", scope)
	}
	if scope := required(Attributes{Action: "delete"}); scope != "" {
		t.Fatalf("unmapped scope = %q", scope)
	}
}

func TestOAuth2ScopeAuthorizer(t *testing.T) {
	authorizer := OAuth2ScopeAuthorizer{
		RequiredScope: func(attributes Attributes) string {
			if attributes.Action == "get" {
				return "orders.read"
			}
			return "orders.write"
		},
	}
	service := UserInfo{Extra: map[string][]string{
		IAMPrincipalTypeExtra: {OAuth2ClientPrincipalType},
		OAuth2ScopeExtra:      {"orders.read"},
	}}
	decision, _, err := authorizer.Authorize(context.Background(), service, Attributes{Action: "get"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionAllow {
		t.Fatalf("read decision = %s", decision)
	}
	decision, reason, err := authorizer.Authorize(context.Background(), service, Attributes{Action: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionDeny || reason != "missing required OAuth 2.0 scope" {
		t.Fatalf("write decision = %s, reason = %q", decision, reason)
	}
	decision, _, err = authorizer.Authorize(context.Background(), UserInfo{Name: "user"}, Attributes{Action: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionNoOpinion {
		t.Fatalf("user decision = %s", decision)
	}
	chain := AuthorizerChain{authorizer, NewAlwaysAllowAuthorizer()}
	decision, _, err = chain.Authorize(context.Background(), UserInfo{Name: "user"}, Attributes{Action: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionAllow {
		t.Fatalf("chained user decision = %s", decision)
	}
}

func TestOAuth2ScopeAuthorizerDeniesUnmappedServiceRequest(t *testing.T) {
	authorizer := OAuth2ScopeAuthorizer{
		RequiredScope: func(Attributes) string { return "" },
	}
	service := UserInfo{Extra: map[string][]string{
		IAMPrincipalTypeExtra: {OAuth2ClientPrincipalType},
		OAuth2ScopeExtra:      {"orders.read"},
	}}
	decision, _, err := authorizer.Authorize(context.Background(), service, Attributes{})
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionDeny {
		t.Fatalf("decision = %s", decision)
	}
}

func TestOAuth2ScopeDenialWritesBearerChallenge(t *testing.T) {
	service := UserInfo{Extra: map[string][]string{
		IAMPrincipalTypeExtra: {OAuth2ClientPrincipalType},
		OAuth2ScopeExtra:      {"orders.read"},
	}}
	authorizer := OAuth2ScopeAuthorizer{
		RequiredScope: func(Attributes) string { return "orders.write" },
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/orders", nil)
	request = request.WithContext(WithResponseHeader(request.Context(), response.Header()))
	request = request.WithContext(WithAuthenticate(request.Context(), AuthenticateInfo{User: service}))
	request = request.WithContext(WithAttributes(request.Context(), &Attributes{Method: http.MethodPost}))

	NewAuthorizationFilter(authorizer).Process(
		response,
		request,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("denied request reached handler")
		}),
	)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := response.Header().Get("WWW-Authenticate"); got != `Bearer error="insufficient_scope"` {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
}
