package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
	"golang.org/x/crypto/ssh"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

type tokenAuthenticatorFunc func(context.Context, string) (*AuthenticationInfo, error)

func (f tokenAuthenticatorFunc) AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error) {
	return f(ctx, token)
}

type basicAuthenticatorFunc func(context.Context, string, string) (*AuthenticationInfo, error)

func (f basicAuthenticatorFunc) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	return f(ctx, username, password)
}

type sshAuthenticatorFuncs struct {
	basic     basicAuthenticatorFunc
	publicKey func(context.Context, ssh.PublicKey) (*AuthenticationInfo, error)
}

func (f sshAuthenticatorFuncs) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	return f.basic(ctx, username, password)
}

func (f sshAuthenticatorFuncs) AuthenticatePublicKey(ctx context.Context, key ssh.PublicKey) (*AuthenticationInfo, error) {
	return f.publicKey(ctx, key)
}

func TestAuthenticatorChainsSkipWrappedErrNotProvided(t *testing.T) {
	wrapped := fmt.Errorf("wrapped: %w", ErrNotProvided)
	if _, err := (TokenAuthenticatorChain{
		tokenAuthenticatorFunc(func(context.Context, string) (*AuthenticationInfo, error) { return nil, wrapped }),
	}).AuthenticateToken(context.Background(), "token"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("TokenAuthenticatorChain error = %v", err)
	}
	if _, err := (BasicAuthenticatorChain{
		basicAuthenticatorFunc(func(context.Context, string, string) (*AuthenticationInfo, error) { return nil, wrapped }),
	}).AuthenticateBasic(context.Background(), "user", "pass"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("BasicAuthenticatorChain error = %v", err)
	}
	sshAuth := sshAuthenticatorFuncs{
		basic:     func(context.Context, string, string) (*AuthenticationInfo, error) { return nil, wrapped },
		publicKey: func(context.Context, ssh.PublicKey) (*AuthenticationInfo, error) { return nil, wrapped },
	}
	if _, err := (SSHAuthenticatorChain{sshAuth}).AuthenticateBasic(context.Background(), "user", "pass"); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("SSHAuthenticatorChain.AuthenticateBasic error = %v", err)
	}
	if _, err := (SSHAuthenticatorChain{sshAuth}).AuthenticatePublicKey(context.Background(), nil); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("SSHAuthenticatorChain.AuthenticatePublicKey error = %v", err)
	}
	if _, err := (AuthenticatorChain{
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) { return nil, wrapped }),
	}).Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("AuthenticatorChain error = %v", err)
	}
}

func TestAuthenticatorChainPreservesFallbackAndRealErrors(t *testing.T) {
	realError := errors.New("backend unavailable")
	want := &AuthenticationInfo{Subject: Subject{ID: "alice"}}
	chain := AuthenticatorChain{
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) { return nil, ErrNotProvided }),
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) { return nil, realError }),
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) { return want, nil }),
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
			AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
				return nil, ErrNotProvided
			}),
		},
		NewAnonymousAuthenticator(),
	)

	got, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.ID != AnonymousSubjectID {
		t.Fatalf("Authenticate() subject = %q, want %q", got.ID, AnonymousSubjectID)
	}
}

func TestFallbackAuthenticatorAllowsLaterAuthenticatorToAcceptRejectedCredential(t *testing.T) {
	rejected := errors.New("OAuth2 credential rejected")
	want := &AuthenticationInfo{Subject: Subject{ID: "webhook-user"}}
	authenticator := NewFallbackAuthenticator(
		AuthenticatorChain{
			AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
				return nil, rejected
			}),
			AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
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
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
			return nil, rejected
		}),
		NewAnonymousAuthenticator(),
	)

	_, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !errors.Is(err, rejected) {
		t.Fatalf("Authenticate() error = %v, want rejected credential", err)
	}
}

func TestAuthenticationFilterTreatsInvalidSuccessfulAuthenticationAsServerError(t *testing.T) {
	filter := NewAuthenticationFilter(
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
			return &AuthenticationInfo{}, nil
		}),
		nil,
	)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called with invalid authentication info")
	}))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestAuthenticationFilterRedactsAndLogsDiagnosticError(t *testing.T) {
	diagnostic := errors.New("LDAP bind failed with password hunter2")
	filter := NewAuthenticationFilter(
		AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
			return nil, diagnostic
		}),
		nil,
	)
	var logOutput strings.Builder
	logger := funcr.New(func(prefix, args string) {
		logOutput.WriteString(prefix)
		logOutput.WriteString(args)
	}, funcr.Options{})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(log.NewContext(request.Context(), logger))
	response := httptest.NewRecorder()

	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected request reached handler")
	}))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if strings.Contains(response.Body.String(), diagnostic.Error()) {
		t.Fatalf("response exposed diagnostic error: %s", response.Body.String())
	}
	if !strings.Contains(logOutput.String(), diagnostic.Error()) {
		t.Fatalf("log did not contain diagnostic error: %s", logOutput.String())
	}
}

func TestAuthenticationFilterWritesAuthenticationChallengeError(t *testing.T) {
	filter := NewAuthenticationFilter(
		AuthenticatorChain{
			AuthenticatorFunc(func(http.ResponseWriter, *http.Request) (*AuthenticationInfo, error) {
				return nil, NewUnauthorizedChallengeError(`Bearer error="invalid_token"`, "Unauthorized")
			}),
		},
		nil,
	)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected request reached handler")
	}))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("response status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if got := response.Header().Get("WWW-Authenticate"); got != `Bearer error="invalid_token"` {
		t.Fatalf("WWW-Authenticate = %q, want invalid_token challenge", got)
	}
}

func TestBearerTokenAuthenticatorRejectsUnrecognizedCredential(t *testing.T) {
	authenticator := NewBearerTokenAuthenticator(tokenAuthenticatorFunc(
		func(context.Context, string) (*AuthenticationInfo, error) {
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
		NewBearerTokenAuthenticator(tokenAuthenticatorFunc(
			func(context.Context, string) (*AuthenticationInfo, error) {
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
		NewBearerTokenAuthenticator(tokenAuthenticatorFunc(
			func(context.Context, string) (*AuthenticationInfo, error) {
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
	authenticator := NewBasicAuthenticator(basicAuthenticatorFunc(
		func(context.Context, string, string) (*AuthenticationInfo, error) {
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
	authenticator := NewSessionAuthenticator(tokenAuthenticatorFunc(
		func(context.Context, string) (*AuthenticationInfo, error) {
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
