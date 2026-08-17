package api

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestCacheTokenAuthenticatorReturnsOwnedAuthentication(t *testing.T) {
	source := &AuthenticationInfo{
		Subject: Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "worker", Groups: []string{"services"}},
		Access:  &AccessConstraints{Audiences: []string{"cloud"}, Scopes: []string{"instances.read"}},
	}
	authenticator := NewCacheTokenAuthenticator(
		tokenAuthenticatorFunc(func(context.Context, string) (*AuthenticationInfo, error) { return source, nil }),
		1,
		time.Minute,
	)

	first, err := authenticator.AuthenticateToken(t.Context(), "token")
	if err != nil {
		t.Fatal(err)
	}
	first.Groups[0] = "changed"
	first.Actor.Groups[0] = "changed"
	first.Access.Audiences[0] = "changed"
	first.Access.Scopes[0] = "changed"

	second, err := authenticator.AuthenticateToken(t.Context(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(second.Groups, []string{"developers"}) ||
		!slices.Equal(second.Actor.Groups, []string{"services"}) ||
		!slices.Equal(second.Access.Audiences, []string{"cloud"}) ||
		!slices.Equal(second.Access.Scopes, []string{"instances.read"}) {
		t.Fatalf("cached authentication was mutated: %#v", second)
	}
}
