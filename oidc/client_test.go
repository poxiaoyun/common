package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

func TestTokenSetFromOAuth2(t *testing.T) {
	set, err := TokenSetFromOAuth2((&oauth2.Token{
		AccessToken:  "access-token",
		TokenType:    "Bearer",
		RefreshToken: "refresh-token",
	}).WithExtra(map[string]any{
		"scope":    "read write",
		"id_token": "unverified-id-token",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if set.AccessToken != "access-token" || strings.Join(set.Scopes, " ") != "read write" {
		t.Fatalf("TokenSet = %#v", set)
	}
	if set.IDToken != nil {
		t.Fatalf("unverified ID Token was populated: %#v", set.IDToken)
	}
	_, err = TokenSetFromOAuth2(&oauth2.Token{
		AccessToken: "access-token",
	})
	if err != nil {
		return
	}
	t.Fatal("TokenSetFromOAuth2 accepted a missing token_type")
}

func TestEndpointTokenError(t *testing.T) {
	converted := EndpointTokenError(&oauth2.RetrieveError{
		Response: &http.Response{
			StatusCode: http.StatusBadRequest,
		},
		ErrorCode:        "invalid_grant",
		ErrorDescription: "grant expired",
		ErrorURI:         "https://provider.example/errors/invalid-grant",
	})
	var endpoint *EndpointError
	if !errors.As(converted, &endpoint) {
		t.Fatalf("error = %T, want *EndpointError", converted)
	}
	if endpoint.Code != "invalid_grant" || endpoint.StatusCode != http.StatusBadRequest {
		t.Fatalf("EndpointError = %#v", endpoint)
	}
	original := errors.New("transport failed")
	if EndpointTokenError(original) != original {
		t.Fatal("non-token endpoint error was replaced")
	}
}

func TestGetClientCredentialsTokenCachesConcurrentRequest(t *testing.T) {
	var calls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/token":
			calls.Add(1)
			clientID, secret, ok := request.BasicAuth()
			if !ok || clientID != "client" || secret != "secret" {
				t.Fatalf("client authentication = %q, %q", clientID, secret)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("grant_type") != "client_credentials" {
				t.Fatalf("grant_type = %q", request.Form.Get("grant_type"))
			}
			if request.Form.Get("resource") != "urn:orders:api" {
				t.Fatalf("resource = %q", request.Form.Get("resource"))
			}
			if request.Form.Get("scope") != "read write" {
				t.Fatalf("scope = %q", request.Form.Get("scope"))
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": "machine-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"scope":        "read write",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
	})
	source := client.NewClientCredentialsTokenSource(ClientCredentialsOptions{
		Resource: "urn:orders:api",
		Scopes:   []string{"read", "write"},
	})
	const callers = 20
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			token, err := source.Token(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			if token.AccessToken != "machine-token" {
				t.Errorf("access token = %q", token.AccessToken)
			}
		}()
	}
	wait.Wait()
	if calls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1", calls.Load())
	}
}

func TestClientCredentialsTokenSourcesShareDiscoveryAndIsolateTokens(t *testing.T) {
	var discoveries atomic.Int32
	var iamTokens atomic.Int32
	var cloudTokens atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveries.Add(1)
			WriteProviderMetadata(t, response, server.URL)
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			resource := request.Form.Get("resource")
			scope := request.Form.Get("scope")
			token := ""
			switch resource {
			case "urn:iam:api":
				iamTokens.Add(1)
				if scope != "create:authentication-reviews" {
					t.Fatalf("IAM scope = %q", scope)
				}
				token = "iam-token"
			case "urn:cloud:api":
				cloudTokens.Add(1)
				if scope != "read:clusters" {
					t.Fatalf("Cloud scope = %q", scope)
				}
				token = "cloud-token"
			default:
				t.Fatalf("resource = %q", resource)
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": token,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "apiserver", ClientSecret: "secret"},
	})
	iamScopes := []string{"create:authentication-reviews"}
	iam := client.NewClientCredentialsTokenSource(ClientCredentialsOptions{Resource: "urn:iam:api", Scopes: iamScopes})
	cloud := client.NewClientCredentialsTokenSource(ClientCredentialsOptions{Resource: "urn:cloud:api", Scopes: []string{"read:clusters"}})
	iamScopes[0] = "mutated"

	iamToken, err := iam.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cloudToken, err := cloud.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if iamToken.AccessToken != "iam-token" || cloudToken.AccessToken != "cloud-token" {
		t.Fatalf("tokens = (%q, %q)", iamToken.AccessToken, cloudToken.AccessToken)
	}
	if _, err := iam.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if discoveries.Load() != 1 || iamTokens.Load() != 1 || cloudTokens.Load() != 1 {
		t.Fatalf("calls = discovery:%d iam:%d cloud:%d", discoveries.Load(), iamTokens.Load(), cloudTokens.Load())
	}
}

func TestAuthorizationCodeAndRefreshTokens(t *testing.T) {
	key := NewSigningKey(t, "key-1")
	atHash, err := TokenHash("access-token", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	var authorization AuthorizationCodeFlow
	var refreshCalls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{key.Public()},
			})
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("grant_type") == "refresh_token" {
				refreshCalls++
				if request.Form.Get("refresh_token") != "refresh-token" {
					t.Fatalf("refresh_token = %q", request.Form.Get("refresh_token"))
				}
				clientID, secret, ok := request.BasicAuth()
				if !ok || clientID != "client" || secret != "secret" {
					t.Fatalf("client authentication = %q, %q", clientID, secret)
				}
				payload := map[string]any{
					"access_token": "refreshed-access-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				}
				if refreshCalls == 2 {
					payload["id_token"] = SignJWT(t, key, "JWT", IDTokenClaims{JWTClaims: JWTClaims{
						Issuer: server.URL, Subject: "other-user", Audience: Audience{"client"},
						Expiry: NewUnixTime(time.Now().Add(time.Hour)), IssuedAt: NewUnixTime(time.Now()),
					}})
				}
				response.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(response).Encode(payload)
				return
			}
			if request.Form.Get("code_verifier") != authorization.CodeVerifier {
				t.Fatalf("code verifier = %q", request.Form.Get("code_verifier"))
			}
			raw := SignJWT(t, key, "JWT", IDTokenClaims{
				JWTClaims: JWTClaims{
					Issuer: server.URL, Subject: "user-1", Audience: Audience{"client"},
					Expiry: NewUnixTime(time.Now().Add(time.Hour)), IssuedAt: NewUnixTime(time.Now()),
				},
				Nonce: authorization.Nonce, AccessTokenHash: atHash,
			})
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token":  "access-token",
				"token_type":    "Bearer",
				"refresh_token": "refresh-token",
				"expires_in":    3600,
				"id_token":      raw,
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
		RedirectURL:    "https://client.example/callback",
		Scopes:         []string{"profile"},
	})
	authorization, err = client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{
		State: "caller-state",
	})
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(authorization.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorizationURL.Query()
	if authorization.State != "caller-state" || query.Get("state") != "caller-state" {
		t.Fatalf("authorization state = %q, URL = %s", authorization.State, authorization.URL)
	}
	if query.Get("scope") != "openid profile" || query.Get("code_challenge_method") != "S256" || query.Get("nonce") != authorization.Nonce {
		t.Fatalf("authorization URL = %s", authorization.URL)
	}
	tokens, err := client.CompleteAuthorizationCodeFlow(context.Background(), url.Values{
		"state": {authorization.State},
		"code":  {"code"},
	}, authorization)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := client.RefreshTokens(context.Background(), tokens)
	if err != nil {
		t.Fatal(err)
	}
	if refreshCalls != 1 || refreshed.AccessToken != "refreshed-access-token" || refreshed.RefreshToken != "refresh-token" {
		t.Fatalf("refreshed tokens = %#v", refreshed)
	}
	if refreshed.IDToken != tokens.IDToken {
		t.Fatal("refresh without ID Token did not retain verified identity")
	}
	if _, err := client.RefreshTokens(context.Background(), refreshed); err != nil {
		if !strings.Contains(err.Error(), "subject changed") {
			t.Fatalf("error = %v", err)
		}
	} else {
		t.Fatal("refresh accepted a different subject")
	}
}

func NewTestClient(t *testing.T, server *httptest.Server, options ClientOptions) *Client {
	t.Helper()
	options.Issuer = server.URL
	options.HTTPClient = server.Client()
	client, err := NewClient(options)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func WriteProviderMetadata(t *testing.T, response http.ResponseWriter, issuer string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(ProviderMetadata(issuer)); err != nil {
		t.Fatal(err)
	}
}

func ProviderMetadata(issuer string) OpenIDProviderMetadata {
	return OpenIDProviderMetadata{
		Issuer:                            issuer,
		AuthorizationEndpoint:             issuer + "/authorize",
		TokenEndpoint:                     issuer + "/token",
		UserInfoEndpoint:                  issuer + "/userinfo",
		JWKSURI:                           issuer + "/jwks",
		ResponseTypesSupported:            []string{"code"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
		UserInfoSigningAlgValuesSupported: []string{"RS256"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token", "client_credentials"},
		RevocationEndpoint:                issuer + "/revoke",
		IntrospectionEndpoint:             issuer + "/introspect",
		EndSessionEndpoint:                issuer + "/logout",
		CodeChallengeMethodsSupported:     []string{"S256"},
	}
}

func NewSigningKey(t *testing.T, keyID string) jose.JSONWebKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return jose.JSONWebKey{
		Key:       key,
		KeyID:     keyID,
		Algorithm: "RS256",
		Use:       "sig",
	}
}

func SignJWT(t *testing.T, key jose.JSONWebKey, tokenType string, claims any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       key,
		},
		(&jose.SignerOptions{}).WithType(jose.ContentType(tokenType)),
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).
		Claims(claims).
		Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
