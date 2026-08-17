package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCredentialsRoundTripper(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if _, exists := request.Form["resource"]; exists {
				t.Fatalf("unexpected resource = %q", request.Form.Get("resource"))
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": "machine-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/resource":
			authorization := request.Header.Get("Authorization")
			if authorization != "Bearer machine-token" {
				t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
			}
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		Issuer:         server.URL,
		HTTPClient:     server.Client(),
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret", Method: ClientAuthSecretBasic},
	})
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{
		Transport: NewClientCredentialsRoundTripper(client, server.Client().Transport),
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if request.Header.Get("Authorization") != "" {
		t.Fatalf("input request was mutated")
	}
	overridden, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	overridden.Header.Set("Authorization", "DPoP caller-token")
	response, err = httpClient.Do(overridden)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if overridden.Header.Get("Authorization") != "DPoP caller-token" {
		t.Fatalf("input request Authorization = %q", overridden.Header.Get("Authorization"))
	}
}
