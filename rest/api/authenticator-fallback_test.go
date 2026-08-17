package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFallbackAuthenticatorUsesFallbackWhenPrimaryDoesNotApply(t *testing.T) {
	want := &AuthenticationInfo{Subject: Subject{ID: AnonymousSubjectID}}
	authenticator := NewFallbackAuthenticator(
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
			return nil, ErrNotProvided
		}),
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
			return want, nil
		}),
	)

	got, err := authenticator.Authenticate(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if err != nil || got != want {
		t.Fatalf("Authenticate() = %#v, %v, want fallback result", got, err)
	}
}
