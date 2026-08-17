package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestKeySetReloadsRotatedKey(t *testing.T) {
	oldKey := NewSigningKey(t, "old")
	newKey := NewSigningKey(t, "new")
	keys := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{oldKey.Public()},
	}
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/jwks":
			requests.Add(1)
			_ = json.NewEncoder(response).Encode(keys)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "apps"},
	})
	claims := IDTokenClaims{JWTClaims: JWTClaims{
		Issuer: server.URL, Subject: "user-1", Audience: Audience{"apps"},
		Expiry: NewUnixTime(time.Now().Add(time.Hour)), IssuedAt: NewUnixTime(time.Now()),
	}}
	if _, err := client.VerifyIDToken(context.Background(), SignJWT(t, oldKey, "JWT", claims), IDTokenChecks{}); err != nil {
		t.Fatal(err)
	}
	keys = jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{newKey.Public()},
	}
	if _, err := client.VerifyIDToken(context.Background(), SignJWT(t, newKey, "JWT", claims), IDTokenChecks{}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("JWKS requests = %d", requests.Load())
	}
}
