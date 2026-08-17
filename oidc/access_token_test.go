package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestClientVerifiesJWTAccessToken(t *testing.T) {
	key := NewSigningKey(t, "key-1")
	var jwksCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			jwksCalls.Add(1)
			_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{key.Public()},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"},
		AccessTokenValidation: AccessTokenValidation{
			Mode:              AccessTokenValidationJWT,
			Audience:          "https://api.example",
			SigningAlgorithms: []string{"RS256"},
		},
	})
	now := time.Now()
	raw := SignJWT(t, key, "at+jwt", JWTAccessTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "service-account", Audience: Audience{"https://api.example"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now), ID: "token-1",
		},
		ClientID: "client", Scope: "read write",
		Actor: &ActorClaims{
			Subject: "current-service",
			Actor:   &ActorClaims{Subject: "prior-service"},
		},
	})
	token, err := client.VerifyAccessToken(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if token.Subject != "service-account" || strings.Join(token.Scopes, " ") != "read write" {
		t.Fatalf("token = %#v", token)
	}
	if token.Actor == nil || token.Actor.Subject != "current-service" || token.Actor.Actor.Subject != "prior-service" {
		t.Fatalf("actor = %#v", token.Actor)
	}
	wrongType := SignJWT(t, key, "JWT", JWTAccessTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "service-account", Audience: Audience{"https://api.example"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now), ID: "token-2",
		},
		ClientID: "client",
	})
	if _, err := client.VerifyAccessToken(context.Background(), wrongType); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("error = %v", err)
	}
	if jwksCalls.Load() != 1 {
		t.Fatalf("JWKS calls = %d, want 1", jwksCalls.Load())
	}
}

func TestClientConfigurationSharesKeySetAcrossTokenVerifiers(t *testing.T) {
	key := NewSigningKey(t, "shared")
	var jwksCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			jwksCalls.Add(1)
			_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{key.Public()}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"},
		AccessTokenValidation: AccessTokenValidation{
			Mode: AccessTokenValidationJWT, Audience: "https://api.example", SigningAlgorithms: []string{"RS256"},
		},
	})
	now := time.Now()
	accessToken := SignJWT(t, key, "at+jwt", JWTAccessTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "service", Audience: Audience{"https://api.example"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now), ID: "access-token",
		},
		ClientID: "client",
	})
	idToken := SignJWT(t, key, "JWT", IDTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "user", Audience: Audience{"client"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now),
		},
	})

	if _, err := client.VerifyAccessToken(context.Background(), accessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VerifyIDToken(context.Background(), idToken, IDTokenChecks{}); err != nil {
		t.Fatal(err)
	}
	if jwksCalls.Load() != 1 {
		t.Fatalf("JWKS calls = %d, want 1", jwksCalls.Load())
	}
}

func TestClientConfigurationRefreshCreatesNewKeySet(t *testing.T) {
	key := NewSigningKey(t, "key")
	var discoveries atomic.Int32
	var jwksCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveries.Add(1)
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			jwksCalls.Add(1)
			_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{key.Public()}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		DiscoveryRefreshInterval: time.Millisecond,
		AccessTokenValidation: AccessTokenValidation{
			Mode: AccessTokenValidationJWT, Audience: "https://api.example", SigningAlgorithms: []string{"RS256"},
		},
	})
	now := time.Now()
	raw := SignJWT(t, key, "at+jwt", JWTAccessTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "service", Audience: Audience{"https://api.example"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now), ID: "access-token",
		},
		ClientID: "client",
	})

	if _, err := client.VerifyAccessToken(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := client.VerifyAccessToken(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for jwksCalls.Load() < 2 {
		if _, err := client.VerifyAccessToken(context.Background(), raw); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("refreshed configuration did not create a new KeySet")
		}
		time.Sleep(time.Millisecond)
	}
	if discoveries.Load() < 2 {
		t.Fatalf("Discovery calls = %d, JWKS calls = %d", discoveries.Load(), jwksCalls.Load())
	}
}

func TestClientIntrospectsOpaqueAccessToken(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/introspect":
			clientID, secret, ok := request.BasicAuth()
			if !ok || clientID != "resource" || secret != "resource-secret" {
				t.Fatalf("introspection credentials = %q, %q", clientID, secret)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("token") != "opaque" {
				t.Fatalf("token = %q", request.Form.Get("token"))
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"active":    true,
				"iss":       server.URL,
				"sub":       "user-1",
				"aud":       []string{"https://api.example"},
				"exp":       time.Now().Add(time.Hour).Unix(),
				"client_id": "client",
				"scope":     "read",
				"act":       map[string]any{"sub": "worker"},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
		AccessTokenValidation: AccessTokenValidation{
			Mode:     AccessTokenValidationIntrospection,
			Audience: "https://api.example",
		},
		IntrospectionAuthentication: &ClientAuthentication{
			ClientID:     "resource",
			ClientSecret: "resource-secret",
			Method:       ClientAuthSecretBasic,
		},
	})
	token, err := client.VerifyAccessToken(context.Background(), "opaque")
	if err != nil {
		t.Fatal(err)
	}
	if token.Subject != "user-1" || token.ClientID != "client" {
		t.Fatalf("token = %#v", token)
	}
	if token.Actor == nil || token.Actor.Subject != "worker" {
		t.Fatalf("actor = %#v", token.Actor)
	}
}

func TestAutoAccessTokenVerifierRoutesWithoutFallback(t *testing.T) {
	jwtError := errors.New("JWT rejected")
	jwtCalls := 0
	introspectionCalls := 0
	verifier := NewAutoAccessTokenVerifier(
		accessTokenVerifierFunc(func(context.Context, string) (*AccessToken, error) {
			jwtCalls++
			return nil, jwtError
		}),
		accessTokenVerifierFunc(func(context.Context, string) (*AccessToken, error) {
			introspectionCalls++
			return &AccessToken{Subject: "introspected"}, nil
		}),
	)

	_, err := verifier.Verify(context.Background(), "eyJhbGciOiJSUzI1NiJ9.payload.signature")
	if !errors.Is(err, jwtError) {
		t.Fatalf("JWT error = %v", err)
	}
	if jwtCalls != 1 || introspectionCalls != 0 {
		t.Fatalf("JWT calls = %d, introspection calls = %d", jwtCalls, introspectionCalls)
	}

	token, err := verifier.Verify(context.Background(), "opaque-token")
	if err != nil {
		t.Fatal(err)
	}
	if token.Subject != "introspected" || jwtCalls != 1 || introspectionCalls != 1 {
		t.Fatalf("token = %#v, JWT calls = %d, introspection calls = %d", token, jwtCalls, introspectionCalls)
	}
}

func TestHasJWTHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "algorithm", header: `{"alg":"RS256"}`, want: true},
		{name: "unsecured algorithm remains on JWT path", header: `{"alg":"none"}`, want: true},
		{name: "missing algorithm", header: `{}`},
		{name: "non-string algorithm", header: `{"alg":1}`},
		{name: "non-object header", header: `[]`},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			header := base64.RawURLEncoding.EncodeToString([]byte(current.header))
			if got := hasJWTHeader(header + ".payload.signature"); got != current.want {
				t.Fatalf("hasJWTHeader() = %t, want %t", got, current.want)
			}
		})
	}
	if hasJWTHeader("not-base64.payload.signature") {
		t.Fatal("hasJWTHeader accepted an invalid protected header")
	}
	if hasJWTHeader("opaque-token") {
		t.Fatal("hasJWTHeader accepted an opaque token")
	}
}

func TestAccessTokenValidationDefaultsToAuto(t *testing.T) {
	options := ClientOptions{
		AccessTokenValidation: AccessTokenValidation{Audience: "https://api.example"},
	}
	metadata := OpenIDProviderMetadata{
		Issuer:                "https://issuer.example",
		JWKSURI:               "https://issuer.example/jwks",
		IntrospectionEndpoint: "https://issuer.example/introspect",
	}
	configuration := newClientConfiguration(options, metadata)
	verifier, err := configuration.GetAccessTokenVerifier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := verifier.(*AutoAccessTokenVerifier); !ok {
		t.Fatalf("verifier = %T", verifier)
	}
	current, err := configuration.GetAccessTokenVerifier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current != verifier {
		t.Fatal("AccessTokenVerifier was rebuilt")
	}
}

type accessTokenVerifierFunc func(context.Context, string) (*AccessToken, error)

func (f accessTokenVerifierFunc) Verify(ctx context.Context, raw string) (*AccessToken, error) {
	return f(ctx, raw)
}
