package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/ssh"
	commonerrors "xiaoshiai.cn/common/errors"
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

func TestFallbackAuthenticatorUsesAnonymousWhenCredentialsAreNotProvided(t *testing.T) {
	authenticator := NewFallbackAuthenticator(
		AuthenticatorChain{
			AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
				return nil, ErrNotProvided
			}),
		},
		NewAnonymousAuthenticator(),
	)

	got, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.User.Name != AnonymousUser {
		t.Fatalf("Authenticate() user = %q, want %q", got.User.Name, AnonymousUser)
	}
}

func TestFallbackAuthenticatorAllowsLaterAuthenticatorToAcceptRejectedCredential(t *testing.T) {
	rejected := errors.New("OAuth2 credential rejected")
	want := &AuthenticateInfo{User: UserInfo{Name: "webhook-user"}}
	authenticator := NewFallbackAuthenticator(
		AuthenticatorChain{
			AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
				return nil, rejected
			}),
			AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
				return want, nil
			}),
		},
		NewAnonymousAuthenticator(),
	)

	got, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || got != want {
		t.Fatalf("Authenticate() = %#v, %v, want later authenticator result", got, err)
	}
}

func TestFallbackAuthenticatorDoesNotReplaceRejectedCredentialsWithAnonymous(t *testing.T) {
	rejected := errors.New("credential rejected")
	authenticator := NewFallbackAuthenticator(
		AuthenticateFunc(func(http.ResponseWriter, *http.Request) (*AuthenticateInfo, error) {
			return nil, rejected
		}),
		NewAnonymousAuthenticator(),
	)

	_, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !errors.Is(err, rejected) {
		t.Fatalf("Authenticate() error = %v, want rejected credential", err)
	}
}

func TestBearerTokenAuthenticatorRejectsUnrecognizedCredential(t *testing.T) {
	authenticator := BearerTokenAuthenticatorWrap(tokenAuthenticatorFunc(
		func(context.Context, string) (*AuthenticateInfo, error) {
			return nil, ErrNotProvided
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer unrecognized-token")

	_, err := authenticator.Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}

func TestFallbackAuthenticatorDoesNotTreatEmptyBearerAsMissingCredential(t *testing.T) {
	authenticator := NewFallbackAuthenticator(
		BearerTokenAuthenticatorWrap(tokenAuthenticatorFunc(
			func(context.Context, string) (*AuthenticateInfo, error) {
				return nil, ErrNotProvided
			},
		)),
		NewAnonymousAuthenticator(),
	)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer ")

	_, err := authenticator.Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}

func TestFallbackAuthenticatorDoesNotTreatBearerSchemeWithoutValueAsMissingCredential(t *testing.T) {
	authenticator := NewFallbackAuthenticator(
		BearerTokenAuthenticatorWrap(tokenAuthenticatorFunc(
			func(context.Context, string) (*AuthenticateInfo, error) {
				return nil, ErrNotProvided
			},
		)),
		NewAnonymousAuthenticator(),
	)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer")

	_, err := authenticator.Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}

func TestWebhookAuthenticatorRejectsEmptyBearerCredential(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer ")

	_, err := (&WebhookAuthenticator{}).Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}

func TestBasicAuthenticatorRejectsUnrecognizedCredential(t *testing.T) {
	authenticator := BasicAuthenticatorWrap(basicAuthenticatorFunc(
		func(context.Context, string, string) (*AuthenticateInfo, error) {
			return nil, ErrNotProvided
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("alice", "incorrect-password")

	_, err := authenticator.Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}

func TestSessionAuthenticatorRejectsUnrecognizedCredential(t *testing.T) {
	authenticator := SessionAuthenticatorWrap(tokenAuthenticatorFunc(
		func(context.Context, string) (*AuthenticateInfo, error) {
			return nil, ErrNotProvided
		},
	), "session")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: "unrecognized-token"})

	_, err := authenticator.Authenticate(httptest.NewRecorder(), request)
	if !commonerrors.IsUnauthorized(err) {
		t.Fatalf("Authenticate() error = %v, want unauthorized", err)
	}
}
