package api

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCacheTokenAuthenticatorReturnsOwnedAuthentication(t *testing.T) {
	source := &Authentication{
		Subject: Subject{ID: "user", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "worker", Groups: []string{"services"}},
		Token:   &TokenInfo{Audiences: []string{"cloud"}, Scopes: []string{"instances.read"}},
	}
	authenticator := NewCacheTokenAuthenticator(
		tokenAuthenticatorFunc(func(context.Context, string) (*Authentication, error) { return source, nil }),
		1,
		time.Minute,
	)

	first, err := authenticator.AuthenticateToken(t.Context(), "token")
	if err != nil {
		t.Fatal(err)
	}
	first.Groups[0] = "changed"
	first.Actor.Groups[0] = "changed"
	first.Token.Audiences[0] = "changed"
	first.Token.Scopes[0] = "changed"

	second, err := authenticator.AuthenticateToken(t.Context(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(second.Groups, []string{"developers"}) ||
		!slices.Equal(second.Actor.Groups, []string{"services"}) ||
		!slices.Equal(second.Token.Audiences, []string{"cloud"}) ||
		!slices.Equal(second.Token.Scopes, []string{"instances.read"}) {
		t.Fatalf("cached authentication was mutated: %#v", second)
	}
	for _, key := range authenticator.Cache.Keys() {
		if strings.Contains(key, "token") {
			t.Fatalf("cache key retained raw token: %q", key)
		}
	}
}

func TestCachedSSHAuthenticatorSeparatesUsersWithoutRetainingPasswords(t *testing.T) {
	calls := 0
	authenticator := NewCachedSSHAuthenticator(sshAuthenticatorFuncs{
		basic: func(_ context.Context, username, _ string) (*Authentication, error) {
			calls++
			return &Authentication{Subject: Subject{ID: username}}, nil
		},
	}, 2, time.Minute)

	alice, err := authenticator.AuthenticateBasic(t.Context(), "alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := authenticator.AuthenticateBasic(t.Context(), "bob", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if alice.ID != "alice" || bob.ID != "bob" || calls != 2 {
		t.Fatalf("authentication = %#v, %#v; calls = %d", alice, bob, calls)
	}
	for _, key := range authenticator.Cache.Keys() {
		if strings.Contains(key, "alice") || strings.Contains(key, "bob") || strings.Contains(key, "secret") {
			t.Fatalf("cache key retained raw SSH credential: %q", key)
		}
	}
}
