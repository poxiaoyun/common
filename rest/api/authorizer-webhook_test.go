package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"xiaoshiai.cn/common/rest/api"
)

func TestWebhookAuthorizerSendsCompleteAuthentication(t *testing.T) {
	authentication := api.AuthenticationInfo{
		Subject: api.Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &api.Subject{ID: "worker"},
		Access:  &api.AccessConstraints{Scopes: []string{"instances.read"}},
	}
	attributes := api.Attributes{Service: "cloud", Action: "get", Path: "/instances/one"}
	request := &api.AuthorizationReview{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(api.AuthorizationReview{Status: &api.AuthorizationReviewStatus{Decision: api.DecisionAllow}})
	}))
	defer server.Close()
	authorizer, err := api.NewWebhookAuthorizer(&api.WebhookAuthorizerOptions{WebhookOptions: api.WebhookOptions{Server: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	decision, _, err := authorizer.Authorize(t.Context(), authentication, attributes)
	if err != nil {
		t.Fatal(err)
	}
	if decision != api.DecisionAllow || request.Spec == nil ||
		!reflect.DeepEqual(request.Spec.Authentication, authentication) ||
		!reflect.DeepEqual(request.Spec.Attributes, attributes) {
		t.Fatalf("request = %#v, decision = %s", request, decision)
	}
}
