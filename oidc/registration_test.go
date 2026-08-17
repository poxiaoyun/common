package oidc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientMetadataJSONPreservesProtocolExtensions(t *testing.T) {
	maxAge := int64(0)
	requireAuthTime := false
	metadata := ClientMetadata{
		ClientName:      "example",
		GrantTypes:      []string{"client_credentials"},
		DefaultMaxAge:   &maxAge,
		RequireAuthTime: &requireAuthTime,
		AdditionalMetadata: map[string]json.RawMessage{
			"client_name#zh-CN": json.RawMessage(`"示例"`),
			"extension":         json.RawMessage(`{"enabled":true}`),
			"client_name":       json.RawMessage(`"must-not-override"`),
		},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	wire := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["client_name"]) != `"example"` {
		t.Fatalf("client_name = %s", wire["client_name"])
	}
	if string(wire["default_max_age"]) != "0" || string(wire["require_auth_time"]) != "false" {
		t.Fatalf("optional zero values = %s", data)
	}
	decoded := ClientMetadata{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientName != "example" || string(decoded.AdditionalMetadata["client_name#zh-CN"]) != `"示例"` {
		t.Fatalf("decoded metadata = %#v", decoded)
	}
	if _, exists := decoded.AdditionalMetadata["client_name"]; exists {
		t.Fatal("standard metadata was retained as an extension")
	}
}

func TestDynamicClientRegistrationLifecycle(t *testing.T) {
	var operation atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(response).Encode(OpenIDProviderMetadata{
				Issuer:               server.URL,
				RegistrationEndpoint: server.URL + "/register",
			})
		case "/register":
			if operation.Add(1) != 1 || request.Method != http.MethodPost {
				t.Fatalf("registration request = %s %s", request.Method, request.URL.Path)
			}
			if request.Header.Get("Authorization") != "Bearer initial-token" {
				t.Fatalf("registration Authorization = %q", request.Header.Get("Authorization"))
			}
			metadata := ClientMetadata{}
			if err := json.NewDecoder(request.Body).Decode(&metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.Scope != "read write" || string(metadata.AdditionalMetadata["client_name#zh-CN"]) != `"计费"` {
				t.Fatalf("registration metadata = %#v", metadata)
			}
			expiresAt := int64(200)
			issuedAt := int64(100)
			response.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(response).Encode(ClientRegistration{
				Metadata:                metadata,
				ClientID:                "client-1",
				ClientSecret:            "secret-1",
				ClientIDIssuedAt:        &issuedAt,
				ClientSecretExpiresAt:   &expiresAt,
				RegistrationAccessToken: "registration-1",
				RegistrationClientURI:   server.URL + "/register/client-1",
			})
		case "/register/client-1":
			switch request.Method {
			case http.MethodGet:
				if operation.Add(1) != 2 || request.Header.Get("Authorization") != "Bearer registration-1" {
					t.Fatalf("read request Authorization = %q", request.Header.Get("Authorization"))
				}
				expiresAt := int64(300)
				_ = json.NewEncoder(response).Encode(ClientRegistration{
					Metadata: ClientMetadata{
						ClientName: "billing",
						Scope:      "read write",
						AdditionalMetadata: map[string]json.RawMessage{
							"client_name#zh-CN": json.RawMessage(`"计费"`),
						},
					},
					ClientID:              "client-1",
					ClientSecret:          "secret-2",
					ClientSecretExpiresAt: &expiresAt,
				})
			case http.MethodPut:
				if operation.Add(1) != 3 || request.Header.Get("Authorization") != "Bearer registration-1" {
					t.Fatalf("update request Authorization = %q", request.Header.Get("Authorization"))
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				payload := map[string]json.RawMessage{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if string(payload["client_id"]) != `"client-1"` || string(payload["client_name"]) != `"billing-v2"` {
					t.Fatalf("update payload = %s", body)
				}
				if _, exists := payload["client_secret"]; exists {
					t.Fatalf("update sent client_secret: %s", body)
				}
				if string(payload["client_name#zh-CN"]) != `"计费"` {
					t.Fatalf("update lost localized metadata: %s", body)
				}
				expiresAt := int64(400)
				_ = json.NewEncoder(response).Encode(ClientRegistration{
					Metadata:                ClientMetadata{ClientName: "billing-v2", Scope: "read"},
					ClientID:                "client-1",
					ClientSecret:            "secret-3",
					ClientSecretExpiresAt:   &expiresAt,
					RegistrationAccessToken: "registration-2",
					RegistrationClientURI:   server.URL + "/register/client-1",
				})
			case http.MethodDelete:
				if operation.Add(1) != 4 || request.Header.Get("Authorization") != "Bearer registration-2" {
					t.Fatalf("delete request Authorization = %q", request.Header.Get("Authorization"))
				}
				response.WriteHeader(http.StatusNoContent)
			default:
				http.NotFound(response, request)
			}
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	current, err := RegisterClient(context.Background(), ClientRegistrationOptions{
		RegistrationEndpoint: server.URL + "/register",
		InitialAccessToken:   "initial-token",
		HTTPClient:           server.Client(),
	}, ClientMetadata{
		ClientName: "billing",
		Scope:      "read write",
		GrantTypes: []string{"client_credentials"},
		AdditionalMetadata: map[string]json.RawMessage{
			"client_name#zh-CN": json.RawMessage(`"计费"`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = GetClientRegistration(context.Background(), current, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if current.ClientSecret != "secret-2" || current.RegistrationAccessToken != "registration-1" {
		t.Fatalf("read registration = %#v", current)
	}
	metadata := current.Metadata
	metadata.ClientName = "billing-v2"
	metadata.Scope = "read"
	metadata.AdditionalMetadata["client_secret"] = json.RawMessage(`"must-not-be-sent"`)
	current, err = UpdateClientRegistration(context.Background(), current, metadata, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if current.ClientSecret != "secret-3" || current.RegistrationAccessToken != "registration-2" {
		t.Fatalf("updated registration = %#v", current)
	}
	if err := DeleteClientRegistration(context.Background(), current, server.Client()); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterClientWithoutInitialAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		expiresAt := int64(0)
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(response).Encode(ClientRegistration{
			ClientID:              "public",
			ClientSecret:          "secret",
			ClientSecretExpiresAt: &expiresAt,
		})
	}))
	defer server.Close()

	_, err := RegisterClient(context.Background(), ClientRegistrationOptions{
		RegistrationEndpoint: server.URL,
		HTTPClient:           server.Client(),
	}, ClientMetadata{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientCredentialsUsesOAuthOnlyProviderMetadata(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(response).Encode(OpenIDProviderMetadata{
				Issuer:                            server.URL,
				TokenEndpoint:                     server.URL + "/token",
				GrantTypesSupported:               []string{"client_credentials"},
				ResponseTypesSupported:            []string{},
				TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
			})
		case "/token":
			clientID, clientSecret, ok := request.BasicAuth()
			if !ok || clientID != "client" || clientSecret != "secret" {
				t.Fatalf("BasicAuth = %q %q %t", clientID, clientSecret, ok)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("grant_type") != "client_credentials" {
				t.Fatalf("token form = %v", request.Form)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		Issuer:         server.URL,
		HTTPClient:     server.Client(),
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.GetClientCredentialsToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "token" || !strings.EqualFold(token.TokenType, "Bearer") {
		t.Fatalf("token = %#v", token)
	}
}
