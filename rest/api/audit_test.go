package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimpleAuditorRecordsSubjectAndActor(t *testing.T) {
	authentication := Authentication{
		Subject: Subject{Type: "iam.user", ID: "user", Name: "Alice", Groups: []string{"developers"}},
		Actor:   &Subject{Type: "iam.workload", ID: "worker", Name: "Worker"},
	}
	request := httptest.NewRequest(http.MethodGet, "/instances/one", nil)
	request = request.WithContext(WithAuthentication(request.Context(), authentication))
	audit := &AuditLog{}
	(&SimpleAuditor{}).OnResponse(httptest.NewRecorder(), request, audit)

	if audit.Subject.Type != "iam.user" || audit.Subject.ID != "user" || audit.Actor == nil ||
		audit.Actor.Type != "iam.workload" || audit.Actor.ID != "worker" {
		t.Fatalf("audit identity = subject %#v, actor %#v", audit.Subject, audit.Actor)
	}
}
