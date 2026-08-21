package oidc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/oidc"
)

func TestClientCredentialsRoundTripper(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"issuer":                                server.URL,
				"authorization_endpoint":                server.URL + "/authorize",
				"token_endpoint":                        server.URL + "/token",
				"jwks_uri":                              server.URL + "/jwks",
				"grant_types_supported":                 []string{"client_credentials"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
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

	client, err := oidc.NewClient(oidc.ClientOptions{
		Issuer:         server.URL,
		HTTPClient:     server.Client(),
		Authentication: oidc.ClientAuthentication{ClientID: "client", ClientSecret: "secret", Method: oidc.ClientAuthSecretBasic},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := client.NewClientCredentialsTokenSource(oidc.ClientCredentialsOptions{})
	httpClient := &http.Client{
		Transport: oidc.NewClientCredentialsRoundTripper(source, server.Client().Transport),
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

func TestClientCredentialsRoundTripperAuthenticatesWebSocket(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"issuer":                                server.URL,
				"authorization_endpoint":                server.URL + "/authorize",
				"token_endpoint":                        server.URL + "/token",
				"jwks_uri":                              server.URL + "/jwks",
				"grant_types_supported":                 []string{"client_credentials"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
		case "/token":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": "websocket-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/events":
			if authorization := request.Header.Get("Authorization"); authorization != "Bearer websocket-token" {
				http.Error(response, "missing websocket authorization", http.StatusUnauthorized)
				return
			}
			connection, err := (&websocket.Upgrader{}).Upgrade(response, request, nil)
			if err != nil {
				t.Errorf("upgrade websocket: %v", err)
				return
			}
			defer connection.Close()
			if err := connection.WriteMessage(websocket.TextMessage, []byte("ready")); err != nil {
				t.Errorf("write websocket message: %v", err)
			}
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	identity, err := oidc.NewClient(oidc.ClientOptions{
		Issuer:         server.URL,
		HTTPClient:     server.Client(),
		Authentication: oidc.ClientAuthentication{ClientID: "client", ClientSecret: "secret", Method: oidc.ClientAuthSecretBasic},
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := httpclient.BuildClientConfig(&httpclient.Options{Server: server.URL}, httpclient.TransportConfig{})
	if err != nil {
		t.Fatal(err)
	}
	config.RoundTripper = oidc.NewClientCredentialsRoundTripper(
		identity.NewClientCredentialsTokenSource(oidc.ClientCredentialsOptions{}),
		config.RoundTripper,
	)
	client, err := httpclient.NewWebSocketClient(config)
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	err = client.Stream(t.Context(), "events", func(_ context.Context, _ int, message []byte) error {
		if got := string(message); got != "ready" {
			t.Errorf("message = %q, want %q", got, "ready")
		}
		return stop
	}, httpclient.WebSocketOptions{})
	if !errors.Is(err, stop) {
		t.Fatalf("Stream() error = %v, want %v", err, stop)
	}
}
