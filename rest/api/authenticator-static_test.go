package api

import (
	"errors"
	"slices"
	"testing"
)

func TestStaticTokenAuthenticatorReturnsFixedAuthentication(t *testing.T) {
	authentication := AuthenticationInfo{
		Subject: Subject{ID: "deployment", Name: "deployment", Groups: []string{"system:admin"}},
		Actor:   &Subject{ID: "operator", Groups: []string{"operators"}},
		Access:  &AccessConstraints{Audiences: []string{"cloud"}, Scopes: []string{"instances.read"}},
	}
	authenticator := NewStaticTokenAuthenticator("deployment-secret", authentication)
	authentication.Groups[0] = "changed"

	info, err := authenticator.AuthenticateToken(t.Context(), "deployment-secret")
	if err != nil {
		t.Fatalf("AuthenticateToken() error = %v", err)
	}
	if info.ID != "deployment" || info.Name != "deployment" || !slices.Equal(info.Groups, []string{"system:admin"}) {
		t.Fatalf("AuthenticateToken() = %#v", info)
	}

	info.Groups[0] = "changed"
	info.Actor.Groups[0] = "changed"
	info.Access.Scopes[0] = "changed"
	again, err := authenticator.AuthenticateToken(t.Context(), "deployment-secret")
	if err != nil {
		t.Fatalf("AuthenticateToken() second error = %v", err)
	}
	if !slices.Equal(again.Groups, []string{"system:admin"}) ||
		!slices.Equal(again.Actor.Groups, []string{"operators"}) ||
		!slices.Equal(again.Access.Scopes, []string{"instances.read"}) {
		t.Fatalf("AuthenticateToken() second = %#v", again)
	}
}

func TestStaticTokenAuthenticatorDoesNotRecognizeOtherTokens(t *testing.T) {
	authenticator := NewStaticTokenAuthenticator("deployment-secret", AuthenticationInfo{Subject: Subject{ID: "deployment"}})
	_, err := authenticator.AuthenticateToken(t.Context(), "another-token")
	if !errors.Is(err, ErrNotProvided) {
		t.Fatalf("AuthenticateToken() error = %v, want ErrNotProvided", err)
	}
}
