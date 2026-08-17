package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWebhookAuthenticatorReturnsCanonicalAuthentication(t *testing.T) {
	want := AuthenticationInfo{
		Subject: Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "worker"},
		Access:  &AccessConstraints{Audiences: []string{"cloud"}, Scopes: []string{"instances.read"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(WebhookAuthenticationResponse{
			Authenticated:  true,
			Authentication: &want,
		})
	}))
	defer server.Close()
	processor, err := NewWebhookAuthenticatorProcessor(&WebhookOptions{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	got, err := processor.Process(t.Context(), &WebhookAuthenticationRequest{Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("authentication = %#v, want %#v", got, &want)
	}
}
