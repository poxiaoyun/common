package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
)

func TestGetUserInfoBindsSubject(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/userinfo":
			if request.Header.Get("Authorization") != "Bearer access-token" {
				t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"sub":  "user-1",
				"name": "Example User",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"},
	})
	info, err := client.GetUserInfo(context.Background(), "access-token", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	claims := struct {
		Name string `json:"name"`
	}{}
	if err := info.DecodeClaims(&claims); err != nil {
		t.Fatal(err)
	}
	if info.Subject != "user-1" || info.Name != "Example User" || claims.Name != "Example User" {
		t.Fatalf("UserInfo = %#v, claims = %#v", info, claims)
	}
}

func TestGetUserInfoVerifiesSignedResponse(t *testing.T) {
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
		case "/userinfo":
			raw := SignJWT(t, key, "JWT", UserInfoClaims{
				JWTClaims: JWTClaims{Issuer: server.URL, Subject: "user-1", Audience: Audience{"client"}},
			})
			response.Header().Set("Content-Type", "application/jwt")
			_, _ = response.Write([]byte(raw))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"},
	})
	info, err := client.GetUserInfo(context.Background(), "access-token", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Subject != "user-1" {
		t.Fatalf("subject = %q", info.Subject)
	}
	if _, err := client.GetUserInfo(context.Background(), "access-token", "user-1"); err != nil {
		t.Fatal(err)
	}
	if jwksCalls.Load() != 1 {
		t.Fatalf("JWKS calls = %d, want 1", jwksCalls.Load())
	}
}
