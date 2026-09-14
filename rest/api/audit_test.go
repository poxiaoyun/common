package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

func TestSimpleAuditorRecordsSubjectAndActor(t *testing.T) {
	authentication := api.Authentication{
		Subject: api.Subject{Type: "iam.user", ID: "user", Name: "Alice", Groups: []string{"developers"}},
		Actor:   &api.Subject{Type: "iam.workload", ID: "worker", Name: "Worker"},
	}
	request := httptest.NewRequest(http.MethodGet, "/instances/one", nil)
	request = request.WithContext(api.WithAuthentication(request.Context(), authentication))
	audit := &api.AuditLog{}
	(&api.SimpleAuditor{}).OnResponse(httptest.NewRecorder(), request, audit)

	if audit.Subject.Type != "iam.user" || audit.Subject.ID != "user" || audit.Actor == nil ||
		audit.Actor.Type != "iam.workload" || audit.Actor.ID != "worker" {
		t.Fatalf("audit identity = subject %#v, actor %#v", audit.Subject, audit.Actor)
	}
}

func TestDefaultAuditFiltersBeforeWebhook(t *testing.T) {
	for _, test := range []struct {
		method string
		want   int32
	}{
		{http.MethodGet, 0}, {http.MethodHead, 0}, {http.MethodOptions, 0},
		{http.MethodPost, 1}, {http.MethodPut, 1}, {http.MethodPatch, 1}, {http.MethodDelete, 1},
	} {
		t.Run(test.method, func(t *testing.T) {
			var calls atomic.Int32
			webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var event api.AuditLog
				if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
					t.Error(err)
				}
				if event.Request.Method != test.method {
					t.Errorf("audit method = %q, want %q", event.Request.Method, test.method)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(webhook.Close)
			sink, err := api.NewWebhookAuditSink(&api.WebhookAuditSinkOptions{
				Options: httpclient.Options{Server: webhook.URL},
			})
			if err != nil {
				t.Fatal(err)
			}
			filter := api.NewSimpleAuditFilter(sink, api.NewDefaultAuditOptions())
			const payload = `{"name":"item"}`
			request := httptest.NewRequest(test.method, "/items", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			body := request.Body
			response := httptest.NewRecorder()
			handled := false
			filter.Process(response, request, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handled = true
				if test.want == 0 && (w != response || r.Body != body) {
					t.Error("excluded request was wrapped for audit capture")
				}
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != payload {
					t.Fatalf("handler body = %q", data)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			if !handled || response.Code != http.StatusNoContent {
				t.Fatalf("handled = %v, status = %d", handled, response.Code)
			}
			if calls.Load() != test.want {
				t.Fatalf("webhook calls = %d, want %d", calls.Load(), test.want)
			}
		})
	}
}

type auditRecorder struct{ events []*api.AuditLog }

func (s *auditRecorder) Save(event *api.AuditLog) error {
	s.events = append(s.events, event)
	return nil
}

func TestAuditMethodSelectionOverride(t *testing.T) {
	for _, test := range []struct {
		name    string
		methods []string
	}{
		{"explicit_read", []string{http.MethodHead}},
		{"all_methods", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := api.NewDefaultAuditOptions()
			options.RecordStatusMethods = test.methods
			sink := &auditRecorder{}
			filter := api.NewSimpleAuditFilter(sink, options)
			filter.Process(httptest.NewRecorder(), httptest.NewRequest(http.MethodHead, "/items", nil),
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
			if len(sink.events) != 1 || sink.events[0].Request.Method != http.MethodHead {
				t.Fatalf("explicit HEAD audit = %+v", sink.events)
			}
		})
	}
}
