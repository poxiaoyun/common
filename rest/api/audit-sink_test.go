package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"xiaoshiai.cn/common/rest/api"
)

type auditRoundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip auditRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type auditSinkFunc func(*api.AuditLog) error

func (save auditSinkFunc) Save(log *api.AuditLog) error {
	return save(log)
}

func TestFanoutAuditSinkSavesToEverySinkInParallel(t *testing.T) {
	firstError := errors.New("first sink failed")
	started := make(chan string, 2)
	release := make(chan struct{})
	releaseOnce := sync.Once{}
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	sink := api.FanoutAuditSink{
		auditSinkFunc(func(*api.AuditLog) error {
			started <- "first"
			<-release
			return firstError
		}),
		auditSinkFunc(func(*api.AuditLog) error {
			started <- "second"
			<-release
			return nil
		}),
	}

	done := make(chan error, 1)
	go func() { done <- sink.Save(&api.AuditLog{}) }()
	called := map[string]bool{}
	for range 2 {
		select {
		case name := <-started:
			called[name] = true
		case <-time.After(time.Second):
			t.Fatal("audit sinks did not start in parallel")
		}
	}
	unblock()
	err := <-done
	if !errors.Is(err, firstError) {
		t.Fatalf("Save() error = %v, want first sink error", err)
	}
	if !called["first"] || !called["second"] {
		t.Fatalf("called sinks = %v, want first and second", called)
	}
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
