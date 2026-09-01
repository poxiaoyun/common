package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFallbackAuthenticatorUsesFallbackWhenPrimaryDoesNotApply(t *testing.T) {
	want := &Authentication{Subject: Subject{ID: AnonymousSubjectID}}
	authenticator := NewFallbackAuthenticator(
		HTTPAuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*Authentication, error) {
			return nil, ErrNotProvided
		}),
		HTTPAuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*Authentication, error) {
			return want, nil
		}),
	)

	got, err := authenticator.AuthenticateHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || got != want {
		t.Fatalf("AuthenticateHTTP() = %#v, %v, want fallback result", got, err)
	}
}
