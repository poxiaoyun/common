package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

type auditRoundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip auditRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestWebhookAuditSinkWrapsTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Service") != "audit" {
			t.Errorf("X-Service = %q", request.Header.Get("X-Service"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sink, err := api.NewWebhookAuditSinkWithTransport(context.Background(), &api.WebhookAuditSinkOptions{
		Server: server.URL,
	}, func(base http.RoundTripper) http.RoundTripper {
		return auditRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			request = request.Clone(request.Context())
			request.Header.Set("X-Service", "audit")
			return base.RoundTrip(request)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Save(&api.AuditLog{}); err != nil {
		t.Fatal(err)
	}
}
