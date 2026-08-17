package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimpleAuditorRecordsSubjectAndActor(t *testing.T) {
	authentication := AuthenticationInfo{
		Subject: Subject{ID: "user", Name: "Alice", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "worker", Name: "Worker"},
	}
	request := httptest.NewRequest(http.MethodGet, "/instances/one", nil)
	request = request.WithContext(WithAuthentication(request.Context(), authentication))
	audit := &AuditLog{}
	(&SimpleAuditor{}).OnResponse(httptest.NewRecorder(), request, audit)

	if audit.Subject.ID != "user" || audit.Actor == nil || audit.Actor.ID != "worker" {
		t.Fatalf("audit identity = subject %#v, actor %#v", audit.Subject, audit.Actor)
	}
}
