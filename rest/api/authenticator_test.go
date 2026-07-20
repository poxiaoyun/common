package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/ssh"
)

type tokenAuthenticatorFunc func(context.Context, string) (*AuthenticateInfo, error)

func (f tokenAuthenticatorFunc) AuthenticateToken(ctx context.Context, token string) (*AuthenticateInfo, error) {
	return f(ctx, token)
}

type basicAuthenticatorFunc func(context.Context, string, string) (*AuthenticateInfo, error)

func (f basicAuthenticatorFunc) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return f(ctx, username, password)
}

type sshAuthenticatorFuncs struct {
	basic     basicAuthenticatorFunc
	publicKey func(context.Context, ssh.PublicKey) (*AuthenticateInfo, error)
}

func (f sshAuthenticatorFuncs) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return f.basic(ctx, username, password)
}

func (f sshAuthenticatorFuncs) AuthenticatePublicKey(ctx context.Context, key ssh.PublicKey) (*AuthenticateInfo, error) {
	return f.publicKey(ctx, key)
}

func TestAuthenticatorChainsSkipWrappedErrNotProvided(t *testing.T) {
	wrapped := fmt.Errorf("wrapped: %w", ErrNotProvided)
	if _, err := (TokenAuthenticatorChain{
		tokenAuthenticatorFunc(func(context.Context, string) (*AuthenticateInfo, error) { return nil, wrapped }),
	}).AuthenticateToken(context.Background(), "token"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("TokenAuthenticatorChain error = %v", err)
	}
	if _, err := (BasicAuthenticatorChain{
		basicAuthenticatorFunc(func(context.Context, string, string) (*AuthenticateInfo, error) { return nil, wrapped }),
	}).AuthenticateBasic(context.Background(), "user", "pass"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("BasicAuthenticatorChain error = %v", err)
	}
	sshAuth := sshAuthenticatorFuncs{
		basic:     func(context.Context, string, string) (*AuthenticateInfo, error) { return nil, wrapped },
		publicKey: func(context.Context, ssh.PublicKey) (*AuthenticateInfo, error) { return nil, wrapped },
	}
	if _, err := (SSHAuthenticatorChain{sshAuth}).AuthenticateBasic(context.Background(), "user", "pass"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("SSHAuthenticatorChain.AuthenticateBasic error = %v", err)
	}
	if _, err := (SSHAuthenticatorChain{sshAuth}).AuthenticatePublicKey(context.Background(), nil); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("SSHAuthenticatorChain.AuthenticatePublicKey error = %v", err)
	}
	if _, err := (AuthenticatorChain{
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) { return nil, wrapped }),
	}).Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("AuthenticatorChain error = %v", err)
	}
}

func TestAuthenticatorChainPreservesFallbackAndRealErrors(t *testing.T) {
	realError := errors.New("backend unavailable")
	want := &AuthenticateInfo{User: UserInfo{Name: "alice"}}
	chain := AuthenticatorChain{
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) { return nil, ErrNotProvided }),
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) { return nil, realError }),
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) { return want, nil }),
	}
	got, err := chain.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || got != want {
		t.Fatalf("Authenticate() = %#v, %v", got, err)
	}

	chain = chain[:2]
	if _, err := chain.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, realError) {
		t.Fatalf("Authenticate() error = %v, want aggregate containing real error", err)
	}
}
