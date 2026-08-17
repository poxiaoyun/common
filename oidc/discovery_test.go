package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientDiscoversOnceThenUsesProviderMetadata(t *testing.T) {
	var discoveries atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/tenant/.well-known/openid-configuration":
			discoveries.Add(1)
			metadata := ProviderMetadata(server.URL + "/tenant")
			metadata.TokenEndpoint = server.URL + "/override-token"
			_ = json.NewEncoder(response).Encode(metadata)
		case "/override-token":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": "overridden",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		Issuer:         server.URL + "/tenant",
		HTTPClient:     server.Client(),
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if discoveries.Load() != 0 {
		t.Fatalf("NewClient made %d Discovery calls", discoveries.Load())
	}
	token, err := client.GetClientCredentialsToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "overridden" {
		t.Fatalf("access token = %q", token.AccessToken)
	}
	if discoveries.Load() != 1 {
		t.Fatalf("discovery calls = %d", discoveries.Load())
	}
}

func TestFailedProviderMetadataRefreshRetainsConfiguration(t *testing.T) {
	var fail atomic.Bool
	refreshAttempted := make(chan struct{})
	var discoveries atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		current := discoveries.Add(1)
		if fail.Load() {
			if current == 2 {
				close(refreshAttempted)
			}
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
			return
		}
		WriteProviderMetadata(t, response, server.URL)
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"}, DiscoveryRefreshInterval: time.Nanosecond,
	})
	initial, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if _, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{}); err != nil {
		t.Fatal(err)
	}
	<-refreshAttempted
	current, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.URL[:strings.Index(current.URL, "?")] != initial.URL[:strings.Index(initial.URL, "?")] {
		t.Fatalf("failed refresh changed authorization endpoint from %q to %q", initial.URL, current.URL)
	}
}

func TestStaleOpenIDProviderMetadataRefreshesInBackground(t *testing.T) {
	var discoveries atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		current := discoveries.Add(1)
		if current == 2 {
			close(refreshStarted)
			<-releaseRefresh
		}
		metadata := ProviderMetadata(server.URL)
		metadata.AuthorizationEndpoint = fmt.Sprintf("%s/authorize-%d", server.URL, current)
		_ = json.NewEncoder(response).Encode(metadata)
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"}, DiscoveryRefreshInterval: 10 * time.Millisecond,
	})
	initial, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	current, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.URL[:strings.Index(current.URL, "?")] != initial.URL[:strings.Index(initial.URL, "?")] {
		t.Fatal("stale read did not return the current configuration")
	}
	<-refreshStarted
	for range 10 {
		current, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if current.URL[:strings.Index(current.URL, "?")] != initial.URL[:strings.Index(initial.URL, "?")] {
			t.Fatal("read during refresh did not return the current configuration")
		}
	}
	close(releaseRefresh)
	deadline := time.Now().Add(time.Second)
	for {
		current, err = client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(current.URL, server.URL+"/authorize-2?") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background refresh did not replace the configuration")
		}
		time.Sleep(time.Millisecond)
	}
	if discoveries.Load() != 2 {
		t.Fatalf("Discovery calls = %d", discoveries.Load())
	}
}

func TestNonPositiveDiscoveryRefreshIntervalDisablesAutomaticRefresh(t *testing.T) {
	for _, interval := range []time.Duration{0, -1} {
		t.Run(interval.String(), func(t *testing.T) {
			var discoveries atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/.well-known/openid-configuration" {
					http.NotFound(response, request)
					return
				}
				discoveries.Add(1)
				WriteProviderMetadata(t, response, server.URL)
			}))
			defer server.Close()

			client := NewTestClient(t, server, ClientOptions{
				Authentication: ClientAuthentication{ClientID: "client"}, DiscoveryRefreshInterval: interval,
			})
			initial, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
			if err != nil {
				t.Fatal(err)
			}
			current, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if current.URL[:strings.Index(current.URL, "?")] != initial.URL[:strings.Index(initial.URL, "?")] {
				t.Fatal("configuration changed")
			}
			if discoveries.Load() != 1 {
				t.Fatalf("Discovery calls = %d", discoveries.Load())
			}
		})
	}
}

func TestNewDefaultClientOptionsEnablesAutomaticRefresh(t *testing.T) {
	options := NewDefaultClientOptions()
	if options.DiscoveryRefreshInterval != DefaultDiscoveryRefreshInterval {
		t.Fatalf("Discovery refresh interval = %s", options.DiscoveryRefreshInterval)
	}
}

func TestDiscoveryRejectsIssuerMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		WriteProviderMetadata(t, response, "https://attacker.example")
	}))
	defer server.Close()

	_, err := DiscoverProviderMetadata(context.Background(), server.URL, server.Client())
	if err != nil {
		if !strings.Contains(err.Error(), "issuer mismatch") {
			t.Fatalf("error = %v", err)
		}
		return
	}
	t.Fatal("Discovery succeeded")
}

func TestNewClientUsesAdvertisedPublicClientAuthentication(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			metadata := OpenIDProviderMetadata{
				Issuer:                            server.URL,
				TokenEndpoint:                     server.URL + "/token",
				TokenEndpointAuthMethodsSupported: []string{"none"},
				GrantTypesSupported:               []string{"client_credentials"},
			}
			_ = json.NewEncoder(response).Encode(metadata)
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("client_id") != "public-client" || request.Form.Get("client_secret") != "" {
				t.Fatalf("token form = %v", request.Form)
			}
			if request.Header.Get("Authorization") != "" {
				t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{
				"access_token": "public-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "public-client", ClientSecret: "must-not-be-sent"},
	})
	if _, err := client.GetClientCredentialsToken(context.Background()); err != nil {
		t.Fatal(err)
	}
}
