package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/ssh"
)

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
	Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error)
}

type TokenAuthenticator interface {
	// Authenticate authenticates the token and returns the authentication info.
	// if can't authenticate, return nil, "reason message", nil
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
	return NewBearerTokenAuthenticationFilterWithErrHandle(authenticator, nil)
}

type AuthenticateErrorHandleFunc func(w http.ResponseWriter, r *http.Request, err error)

func NewBearerTokenAuthenticationFilterWithErrHandle(authenticator TokenAuthenticator, errhandle AuthenticateErrorHandleFunc) Filter {
	authfunc := func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtracBearerTokenFromRequest(r)
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authenticator.Authenticate(ctx, token)
	}
	return NewAuthenticateFilter(AuthenticateFunc(authfunc), errhandle)
}

func ResponseHeaderFromContext(ctx context.Context) http.Header {
	return GetContextValue[http.Header](ctx, "response-header")
}

func WithResponseHeader(ctx context.Context, header http.Header) context.Context {
	return SetContextValue(ctx, "response-header", header)
}

// NewCookieTokenAuthenticationFilter get token from cookie
func NewCookieTokenAuthenticationFilter(authenticator TokenAuthenticator, cookieName string) Filter {
	authfunc := func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtracTokenFromCookie(r, cookieName)
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authenticator.Authenticate(ctx, token)
	}
	return NewAuthenticateFilter(AuthenticateFunc(authfunc), nil)
}

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

func ExtracTokenFromCookie(r *http.Request, cookieName string) string {
	cookie, _ := r.Cookie(cookieName)
	if cookie != nil && cookie.Value != "" {
		return cookie.Value
	}
	// fallback
	return ExtracBearerTokenFromRequest(r)
}

func ExtracBearerTokenFromRequest(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if token != "" {
		return strings.TrimPrefix(token, "Bearer ")
	}
	token = r.URL.Query().Get("token")
	return token
}
