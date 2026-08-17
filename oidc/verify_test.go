package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestClientVerifiesIDTokenAuthorizationBinding(t *testing.T) {
	key := NewSigningKey(t, "key-1")
	client, server := NewIDTokenTestClient(t, key, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "apps"},
	})
	defer server.Close()
	atHash, err := TokenHash("access-token", "RS256")
	if err != nil {
		t.Fatal(err)
	}

	emailVerified := true
	now := time.Now()
	raw := SignJWT(t, key, "JWT", IDTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "user-1", Audience: Audience{"apps"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now),
		},
		Nonce: "nonce", AuthenticationContextClassReference: "urn:example:loa:2",
		AuthenticationMethodsReferences: []string{"pwd", "otp"}, AccessTokenHash: atHash,
		AuthorizationCodeHash: "authorization-code-hash", Name: "Example User",
		Email: "user@example.com", EmailVerified: &emailVerified,
		Address: &AddressClaim{Locality: "Shanghai", Country: "CN"}, UpdatedAt: NewUnixTime(now),
	})
	token, err := client.VerifyIDToken(context.Background(), raw, IDTokenChecks{
		Nonce:       "nonce",
		AccessToken: "access-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.Subject != "user-1" {
		t.Fatalf("token = %#v", token)
	}
	if token.AuthenticationContextClassReference != "urn:example:loa:2" || strings.Join(token.AuthenticationMethodsReferences, " ") != "pwd otp" {
		t.Fatalf("authentication context = %#v", token)
	}
	if token.AccessTokenHash != atHash || token.AuthorizationCodeHash != "authorization-code-hash" {
		t.Fatalf("token hashes = %#v", token)
	}
	if token.Name != "Example User" || token.Email != "user@example.com" || token.EmailVerified == nil || !*token.EmailVerified {
		t.Fatalf("standard claims = %#v", token)
	}
	if token.Address == nil || token.Address.Locality != "Shanghai" || token.UpdatedAt == nil {
		t.Fatalf("address or updated time = %#v", token)
	}
	if _, err := client.VerifyIDToken(context.Background(), raw, IDTokenChecks{Nonce: "wrong"}); err != nil {
		if !strings.Contains(err.Error(), "nonce") {
			t.Fatalf("error = %v", err)
		}
	} else {
		t.Fatal("VerifyIDToken succeeded")
	}
}

func TestTokenHashAndVerifyHash(t *testing.T) {
	expected, err := TokenHash("access-token", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyHash("access-token", expected, "RS256"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyHash("other-token", expected, "RS256"); err == nil {
		t.Fatal("VerifyHash accepted a different token")
	}
}

func TestClientAcceptsConfiguredIDTokenAudience(t *testing.T) {
	key := NewSigningKey(t, "key-1")
	client, server := NewIDTokenTestClient(t, key, ClientOptions{
		Authentication:   ClientAuthentication{ClientID: "apps"},
		IDTokenAudiences: []string{"frontend"},
	})
	defer server.Close()

	now := time.Now()
	raw := SignJWT(t, key, "JWT", IDTokenClaims{
		JWTClaims: JWTClaims{
			Issuer: server.URL, Subject: "user-1", Audience: Audience{"frontend"},
			Expiry: NewUnixTime(now.Add(time.Hour)), IssuedAt: NewUnixTime(now),
		},
		AuthorizedParty: "frontend",
	})
	if _, err := client.VerifyIDToken(context.Background(), raw, IDTokenChecks{}); err != nil {
		t.Fatal(err)
	}
}

func TestClientVerifiesClientSecretIDToken(t *testing.T) {
	secret := "01234567890123456789012345678901"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		metadata := OpenIDProviderMetadata{
			Issuer:                           server.URL,
			AuthorizationEndpoint:            server.URL + "/authorize",
			TokenEndpoint:                    server.URL + "/token",
			JWKSURI:                          server.URL + "/jwks",
			ResponseTypesSupported:           []string{"code"},
			SubjectTypesSupported:            []string{"public"},
			IDTokenSigningAlgValuesSupported: []string{"HS256"},
		}
		_ = json.NewEncoder(response).Encode(metadata)
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "apps", ClientSecret: secret},
	})
	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.HS256,
			Key:       []byte(secret),
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).
		Claims(IDTokenClaims{
			JWTClaims: JWTClaims{
				Issuer: server.URL, Subject: "user-1", Audience: Audience{"apps"},
				Expiry: NewUnixTime(time.Now().Add(time.Hour)), IssuedAt: NewUnixTime(time.Now()),
			},
		}).
		Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VerifyIDToken(context.Background(), raw, IDTokenChecks{}); err != nil {
		t.Fatal(err)
	}
}

func NewIDTokenTestClient(t *testing.T, key jose.JSONWebKey, options ClientOptions) (*Client, *httptest.Server) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			_ = json.NewEncoder(response).Encode(jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{key.Public()},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	return NewTestClient(t, server, options), server
}
