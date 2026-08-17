package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWebhookAuthorizerSendsCompleteAuthentication(t *testing.T) {
	authentication := AuthenticationInfo{
		Subject: Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "worker"},
		Access:  &AccessConstraints{Scopes: []string{"instances.read"}},
	}
	attributes := Attributes{Service: "cloud", Action: "get", Path: "/instances/one"}
	var request WebhookAuthorizationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(WebhookAuthorizationResponse{Decision: DecisionAllow})
	}))
	defer server.Close()
	authorizer, err := NewWebhookAuthorizer(&WebhookAuthorizerOptions{WebhookOptions: WebhookOptions{Server: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	decision, _, err := authorizer.Authorize(t.Context(), authentication, attributes)
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionAllow || !reflect.DeepEqual(request.Authentication, authentication) || !reflect.DeepEqual(request.Attributes, attributes) {
		t.Fatalf("request = %#v, decision = %s", request, decision)
	}
}
