package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"xiaoshiai.cn/common/oidc"
)

func TestOAuth2AccessTokenAuthenticator(t *testing.T) {
	var discoveryCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryCalls.Add(1)
			_ = json.NewEncoder(response).Encode(oidc.OpenIDProviderMetadata{
				Issuer:                server.URL,
				IntrospectionEndpoint: server.URL + "/introspect",
				IntrospectionEndpointAuthMethodsSupported: []string{string(oidc.ClientAuthSecretBasic)},
			})
		case "/introspect":
			clientID, secret, ok := request.BasicAuth()
			if !ok || clientID != "orders-api" || secret != "resource-secret" {
				t.Fatalf("introspection credentials = %q, %q", clientID, secret)
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"active":    true,
				"iss":       server.URL,
				"sub":       "service-client",
				"aud":       []string{"urn:orders:api", "urn:other:api"},
				"exp":       time.Now().Add(time.Hour).Unix(),
				"client_id": "service-client",
				"scope":     "orders.read orders.write",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	options := oidc.NewDefaultClientOptions()
	options.Issuer = server.URL
	options.HTTPClient = server.Client()
	options.AccessTokenValidation = oidc.AccessTokenValidation{
		Mode:     oidc.AccessTokenValidationIntrospection,
		Audience: "urn:orders:api",
	}
	options.IntrospectionAuthentication = &oidc.ClientAuthentication{
		ClientID:     "orders-api",
		ClientSecret: "resource-secret",
		Method:       oidc.ClientAuthSecretBasic,
	}
	client, err := oidc.NewClient(options)
	if err != nil {
		t.Fatal(err)
	}
	authenticator := NewOAuth2AccessTokenAuthenticator(client)
	if discoveryCalls.Load() != 0 {
		t.Fatal("constructor performed Provider Discovery")
	}
	info, err := authenticator.AuthenticateToken(context.Background(), "access-token")
	if err != nil {
		t.Fatal(err)
	}
	if discoveryCalls.Load() != 1 {
		t.Fatalf("Discovery calls = %d, want 1", discoveryCalls.Load())
	}
	if !slices.Equal(info.Audiences, []string{"urn:orders:api", "urn:other:api"}) {
		t.Fatalf("audiences = %v", info.Audiences)
	}
	if info.User.ID != "service-client" || info.User.Name != server.URL+"#service-client" {
		t.Fatalf("user = %#v", info.User)
	}
	if !slices.Equal(info.User.Extra[IAMPrincipalTypeExtra], []string{OAuth2ClientPrincipalType}) ||
		!slices.Equal(info.User.Extra[OAuth2ClientIDExtra], []string{"service-client"}) ||
		!slices.Equal(info.User.Extra[OAuth2ScopeExtra], []string{"orders.read", "orders.write"}) {
		t.Fatalf("extra = %#v", info.User.Extra)
	}
}

func TestOAuth2BearerAuthenticationError(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		challenge string
	}{
		{name: "missing", err: ErrNotProvided, challenge: "Bearer"},
		{name: "invalid", err: oidc.ErrInvalidAccessToken, challenge: `Bearer error="invalid_token"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			OAuth2BearerAuthenticationError(
				response,
				httptest.NewRequest(http.MethodGet, "/resource", nil),
				test.err,
			)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if got := response.Header().Get("WWW-Authenticate"); got != test.challenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, test.challenge)
			}
		})
	}
}
