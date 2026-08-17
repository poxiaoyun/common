package api

import (
	"errors"
	"slices"
	"testing"
)

func TestStaticTokenAuthenticatorReturnsFixedIdentity(t *testing.T) {
	user := UserInfo{ID: "deployment", Name: "deployment", Groups: []string{"system:admin"}}
	authenticator := NewStaticTokenAuthenticator("deployment-secret", user)
	user.Groups[0] = "changed"

	info, err := authenticator.AuthenticateToken(t.Context(), "deployment-secret")
	if err != nil {
		t.Fatalf("AuthenticateToken() error = %v", err)
	}
	if info.User.ID != "deployment" || info.User.Name != "deployment" || !slices.Equal(info.User.Groups, []string{"system:admin"}) {
		t.Fatalf("AuthenticateToken() user = %#v", info.User)
	}

	info.User.Groups[0] = "changed"
	again, err := authenticator.AuthenticateToken(t.Context(), "deployment-secret")
	if err != nil {
		t.Fatalf("AuthenticateToken() second error = %v", err)
	}
	if !slices.Equal(again.User.Groups, []string{"system:admin"}) {
		t.Fatalf("AuthenticateToken() second groups = %#v", again.User.Groups)
	}
}

func TestStaticTokenAuthenticatorDoesNotRecognizeOtherTokens(t *testing.T) {
	authenticator := NewStaticTokenAuthenticator("deployment-secret", UserInfo{Name: "deployment"})
	_, err := authenticator.AuthenticateToken(t.Context(), "another-token")
	if !errors.Is(err, ErrNotProvided) {
		t.Fatalf("AuthenticateToken() error = %v, want ErrNotProvided", err)
	}
}
