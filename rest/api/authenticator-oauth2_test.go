package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"xiaoshiai.cn/common/log"
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
			if request.FormValue("token") == "invalid-access-token" {
				_ = json.NewEncoder(response).Encode(map[string]any{"active": false})
				return
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"active":    true,
				"iss":       server.URL,
				"sub":       "service-client",
				"aud":       []string{"urn:orders:api", "urn:other:api"},
				"exp":       time.Now().Add(time.Hour).Unix(),
				"client_id": "service-client",
				"scope":     "orders.read orders.write",
				"username":  "Orders Worker",
				"act":       map[string]any{"sub": "gateway"},
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
	if info.Access == nil || !slices.Equal(info.Access.Audiences, []string{"urn:orders:api", "urn:other:api"}) {
		t.Fatalf("access = %#v", info.Access)
	}
	if info.ID != "service-client" || info.Name != "Orders Worker" {
		t.Fatalf("subject = %#v", info.Subject)
	}
	if !slices.Equal(info.Access.Scopes, []string{"orders.read", "orders.write"}) {
		t.Fatalf("scopes = %#v", info.Access.Scopes)
	}
	if info.Actor == nil || info.Actor.ID != "gateway" {
		t.Fatalf("actor = %#v", info.Actor)
	}

	var logOutput strings.Builder
	logger := funcr.New(func(prefix, args string) {
		logOutput.WriteString(prefix)
		logOutput.WriteString(args)
	}, funcr.Options{})
	ctx := log.NewContext(context.Background(), logger)
	_, err = authenticator.AuthenticateToken(ctx, "invalid-access-token")
	var challengeErr *AuthenticationChallengeError
	if !errors.As(err, &challengeErr) {
		t.Fatalf("AuthenticateToken() error = %v, want AuthenticationChallengeError", err)
	}
	if challengeErr.Challenge != `Bearer error="invalid_token"` {
		t.Fatalf("challenge = %q, want invalid_token", challengeErr.Challenge)
	}
	if !strings.Contains(logOutput.String(), oidc.ErrInvalidAccessToken.Error()) {
		t.Fatalf("log did not contain token validation error: %s", logOutput.String())
	}
}

func TestBearerTokenAuthenticationError(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		challenge string
	}{
		{name: "missing", err: ErrNotProvided, challenge: "Bearer"},
		{
			name:      "challenged",
			err:       NewUnauthorizedChallengeError(`Bearer error="invalid_token"`, "Unauthorized"),
			challenge: `Bearer error="invalid_token"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			BearerTokenAuthenticationError(
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

func TestBearerTokenAuthenticationFilterWritesOAuth2Challenge(t *testing.T) {
	for _, test := range []struct {
		name          string
		authorization string
		err           error
		challenge     string
	}{
		{name: "missing", challenge: "Bearer"},
		{
			name:          "invalid",
			authorization: "Bearer invalid-token",
			err:           NewUnauthorizedChallengeError(`Bearer error="invalid_token"`, "Unauthorized"),
			challenge:     `Bearer error="invalid_token"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter := NewBearerTokenAuthenticationFilter(TokenAuthenticatorChain{
				tokenAuthenticatorFunc(func(context.Context, string) (*AuthenticationInfo, error) {
					return nil, test.err
				}),
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/resource", nil)
			request.Header.Set("Authorization", test.authorization)

			filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler was called")
			}))

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if got := response.Header().Get("WWW-Authenticate"); got != test.challenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, test.challenge)
			}
		})
	}
}
