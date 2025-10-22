package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/errors"
)

var ErrNotProvided = fmt.Errorf("no authentication provided")

const AnonymousUser = "anonymous" // anonymous username

type UserInfo struct {
	ID            string              `json:"id,omitempty"`
	Name          string              `json:"name,omitempty"`
	Email         string              `json:"email,omitempty"`
	EmailVerified bool                `json:"email_verified,omitempty"`
	Groups        []string            `json:"groups,omitempty"`
	Extra         map[string][]string `json:"extra,omitempty"`
}

type Authenticator interface {
	// Authenticate authenticates the request and returns the authentication info.
	// it can has side effect to set response header
	// if implementation can't make authentication decision, return nil, [ErrNotProvided]
	// so that the next authenticator in chain can try
	// once authenticated, return the AuthenticateInfo, nil
	Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error)
}

type TokenAuthenticator interface {
	// Authenticate authenticates the token and returns the authentication info.
	// if unauthorized, return nil, err
	// if no decision can be made, return nil, [ErrNotProvided]
	// if unexpected error, return nil, "", err
	Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error)
}

type BasicAuthenticator interface {
	Authenticate(ctx context.Context, username, password string) (*AuthenticateInfo, error)
}

type AuthenticateInfo struct {
	// Audiences is the set of audiences the authenticator was able to validate
	// the token against. If the authenticator is not audience aware, this field
	// will be empty.
	Audiences []string
	// User is the UserInfo associated with the authentication context.
	User UserInfo
}

type SSHAuthenticator interface {
	BasicAuthenticator
	AuthenticatePublibcKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error)
}

func WithAuthenticate(ctx context.Context, info AuthenticateInfo) context.Context {
	return SetContextValue(ctx, "userinfo", info)
}

func AuthenticateFromContext(ctx context.Context) AuthenticateInfo {
	return GetContextValue[AuthenticateInfo](ctx, "userinfo")
}

func NewBearerTokenAuthenticationFilter(authenticator TokenAuthenticator) Filter {
	return NewAuthenticateFilter(BearerTokenAuthenticatorWrap(authenticator), nil)
}

func ResponseHeaderFromContext(ctx context.Context) http.Header {
	return GetContextValue[http.Header](ctx, "response-header")
}

func WithResponseHeader(ctx context.Context, header http.Header) context.Context {
	return SetContextValue(ctx, "response-header", header)
}

func SessionAuthenticatorWrap(authn TokenAuthenticator, sessionkey string) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtractTokenFromCookie(r, sessionkey)
		if token == "" {
			return nil, ErrNotProvided
		}
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authn.Authenticate(ctx, token)
	})
}

func BearerTokenAuthenticatorWrap(authn TokenAuthenticator) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtractBearerTokenFromRequest(r)
		if token == "" {
			return nil, ErrNotProvided
		}
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authn.Authenticate(ctx, token)
	})
}

func BasicAuthenticatorWrap(authn BasicAuthenticator) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		username, password, ok := r.BasicAuth()
		if !ok {
			return nil, ErrNotProvided
		}
		return authn.Authenticate(r.Context(), username, password)
	})
}

type DelegateAuthenticator []Authenticator

func (d DelegateAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	var errs []error
	for _, a := range d {
		info, err := a.Authenticate(w, r)
		if err == nil {
			return info, nil
		}
		if err == ErrNotProvided {
			continue
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errs)
}

type AuthenticateErrorHandleFunc func(w http.ResponseWriter, r *http.Request, err error)

func NewAuthenticateFilter(authn Authenticator, onerr AuthenticateErrorHandleFunc) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		info, err := authn.Authenticate(w, r)
		if err != nil {
			if onerr != nil {
				onerr(w, r, err)
			} else {
				Unauthorized(w, fmt.Sprintf("Unauthorized: %v", err))
			}
			return
		}
		sp := trace.SpanFromContext(r.Context())
		sp.SetAttributes(
			attribute.String("user.name", info.User.Name),
			attribute.String("user.email", info.User.Email),
		)
		next.ServeHTTP(w, r.WithContext(WithAuthenticate(r.Context(), *info)))
	})
}

func ExtractTokenFromCookie(r *http.Request, cookieName string) string {
	cookie, _ := r.Cookie(cookieName)
	if cookie != nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

func ExtractBearerTokenFromRequest(r *http.Request) string {
	token := r.Header.Get("Authorization")
	// only support bearer token
	if strings.HasPrefix(token, "Bearer ") {
		return strings.TrimPrefix(token, "Bearer ")
	}
	return r.URL.Query().Get("token")
}
