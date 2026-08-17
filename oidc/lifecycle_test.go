package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRevokeTokenAndRPInitiatedLogout(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			WriteProviderMetadata(t, response, server.URL)
		case "/revoke":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("token") != "refresh-token" || request.Form.Get("token_type_hint") != "refresh_token" {
				t.Fatalf("revocation form = %v", request.Form)
			}
			response.WriteHeader(http.StatusOK)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client", ClientSecret: "secret"},
	})
	if err := client.RevokeToken(context.Background(), "refresh-token", TokenTypeRefreshToken); err != nil {
		t.Fatal(err)
	}
	logout, err := client.BeginRPInitiatedLogout(context.Background(), &IDToken{
		Raw: "id-token",
	}, "https://client.example/logout/callback")
	if err != nil {
		t.Fatal(err)
	}
	logoutURL, err := url.Parse(logout.URL)
	if err != nil {
		t.Fatal(err)
	}
	if logoutURL.Query().Get("id_token_hint") != "id-token" || logoutURL.Query().Get("state") != logout.State {
		t.Fatalf("logout URL = %s", logout.URL)
	}
	if err := client.CompleteRPInitiatedLogout(url.Values{"state": {"wrong"}}, logout); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("error = %v", err)
	}
	if err := client.CompleteRPInitiatedLogout(url.Values{"state": {logout.State}}, logout); err != nil {
		t.Fatal(err)
	}
}
