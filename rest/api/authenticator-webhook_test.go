package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

type webhookRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f webhookRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestWebhookAuthenticatorReturnsCanonicalAuthentication(t *testing.T) {
	want := api.AuthenticationInfo{
		Subject: api.Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &api.Subject{ID: "worker"},
		Access:  &api.AccessConstraints{Audiences: []string{"cloud"}, Scopes: []string{"instances.read"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		review := &api.AuthenticationReview{}
		if err := json.NewDecoder(r.Body).Decode(review); err != nil || review.Spec == nil || review.Spec.Token != "token" {
			http.Error(w, "invalid review", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(api.AuthenticationReview{Status: &api.AuthenticationReviewStatus{
			Authenticated:  true,
			Authentication: &want,
		}})
	}))
	defer server.Close()
	processor, err := api.NewWebhookAuthenticatorProcessor(&httpclient.Options{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	got, err := processor.Process(t.Context(), &api.AuthenticationReviewSpec{Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("authentication = %#v, want %#v", got, &want)
	}
}

func TestWebhookAuthenticatorValidatesRequestedAudienceAndWrapsTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "missing service token", http.StatusUnauthorized)
			return
		}
		review := &api.AuthenticationReview{}
		if err := json.NewDecoder(r.Body).Decode(review); err != nil ||
			review.Spec == nil || !reflect.DeepEqual(review.Spec.Audiences, []string{"urn:apps:api"}) {
			http.Error(w, "invalid audiences", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(api.AuthenticationReview{Status: &api.AuthenticationReviewStatus{
			Authenticated:  true,
			Authentication: &api.AuthenticationInfo{Subject: api.Subject{ID: "user"}},
			Audiences:      []string{"urn:apps:api"},
		}})
	}))
	defer server.Close()

	authenticator, err := api.NewWebhookAuthenticatorWithTransport(
		&api.WebhookAuthenticatorOptions{
			Options:   httpclient.Options{Server: server.URL},
			Audiences: []string{"urn:apps:api"},
		},
		func(base http.RoundTripper) http.RoundTripper {
			return webhookRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				request = request.Clone(request.Context())
				request.Header.Set("Authorization", "Bearer service-token")
				return base.RoundTrip(request)
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.AuthenticateToken(t.Context(), "user-token"); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookAuthenticatorRejectsMissingRequestedAudience(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.AuthenticationReview{Status: &api.AuthenticationReviewStatus{
			Authenticated:  true,
			Authentication: &api.AuthenticationInfo{Subject: api.Subject{ID: "user"}},
		}})
	}))
	defer server.Close()
	authenticator, err := api.NewWebhookAuthenticator(&api.WebhookAuthenticatorOptions{
		Options:   httpclient.Options{Server: server.URL},
		Audiences: []string{"urn:apps:api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.AuthenticateToken(t.Context(), "user-token"); err == nil {
		t.Fatal("authentication review without a compatible audience was accepted")
	}
}
