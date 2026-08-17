package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFallbackAuthenticatorUsesFallbackWhenPrimaryDoesNotApply(t *testing.T) {
	want := &AuthenticateInfo{User: UserInfo{Name: AnonymousUser}}
	authenticator := NewFallbackAuthenticator(
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
			return nil, ErrNotProvided
		}),
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
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
