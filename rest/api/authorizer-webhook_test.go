package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

func TestWebhookAuthorizerSendsCompleteAuthentication(t *testing.T) {
	authentication := api.Authentication{
		Subject: api.Subject{Type: "iam.user", ID: "user", Groups: []string{"developers"}},
		Actor:   &api.Subject{Type: "iam.workload", ID: "worker"},
		Token:   &api.TokenInfo{Scopes: []string{"instances.read"}},
	}
	attributes := api.Attributes{Service: "cloud", Action: "get", Path: "/instances/one"}
	request := &api.AuthorizationReview{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(api.AuthorizationReview{Status: &api.AuthorizationReviewStatus{Decision: authz.DecisionAllow}})
	}))
	defer server.Close()
	authorizer, err := api.NewWebhookAuthorizer(&api.WebhookAuthorizerOptions{Options: httpclient.Options{Server: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	filter := api.NewAuthorizationFilter(authorizer)
	httpRequest := httptest.NewRequest(http.MethodGet, attributes.Path, nil)
	httpRequest = httpRequest.WithContext(api.WithAuthentication(httpRequest.Context(), authentication))
	httpRequest = httpRequest.WithContext(api.WithAttributes(httpRequest.Context(), &attributes))
	response := httptest.NewRecorder()
	filter.Process(response, httpRequest, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	if response.Code != http.StatusOK || request.Spec == nil ||
		!reflect.DeepEqual(request.Spec.Authentication, authentication) ||
		!reflect.DeepEqual(request.Spec.Attributes, attributes) {
		t.Fatalf("request = %#v, status = %d", request, response.Code)
	}
}
