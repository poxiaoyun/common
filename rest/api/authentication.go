package api

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/ssh"
)

// ErrNotProvided means an authenticator did not claim its input. At the request
// seam it means the request contained no applicable credential. Credential
// adapters may use it to let another adapter inspect the same credential, but
// must translate chain exhaustion into an authentication error before returning
// to the request seam.
var ErrNotProvided = fmt.Errorf("no authentication provided")

const AnonymousUser = "anonymous" // anonymous username

// UserInfo is the canonical identity produced by an Authenticator and consumed
// by Authorizers. It may represent either a person or a service principal.
type UserInfo struct {
	// ID is the stable subject identifier assigned by the authentication
	// provider.
	ID string `json:"id,omitempty"`
	// Name uniquely identifies the principal within this API's authentication
	// domain and is the primary value used by authorization and audit records.
	Name string `json:"name,omitempty"`
	// Email is the authenticated principal's email address when the
	// authentication method provides one.
	Email string `json:"email,omitempty"`
	// EmailVerified reports whether the authentication provider verified Email.
	EmailVerified bool `json:"email_verified,omitempty"`
	// Groups contains the authorization groups assigned to the principal.
	Groups []string `json:"groups,omitempty"`
	// Extra carries namespaced authentication attributes consumed by
	// Authorizers, such as OAuth 2.0 client IDs and scopes. Values may be sent to
	// trusted authentication webhooks and request-header proxies and must not
	// contain credentials.
	Extra map[string][]string `json:"extra,omitempty"`
}

type Authenticator interface {
	// Authenticate authenticates the request and returns the authentication info.
	// it can has side effect to set response header
	// if the request has no applicable credential, return nil, [ErrNotProvided]
	// without response side effects so that another authenticator or fallback can run
	// once authenticated, return the AuthenticateInfo, nil
	Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error)
}

type TokenAuthenticator interface {
	// AuthenticateToken authenticates the token and returns the authentication info.
	// if unauthorized, return nil, err
	// if no decision can be made, return nil, [ErrNotProvided]
	// if unexpected error, return nil, "", err
	AuthenticateToken(ctx context.Context, token string) (*AuthenticateInfo, error)
}

type BasicAuthenticator interface {
	// AuthenticateBasic authenticates the username and password and returns the authentication info.
	// It also use for APIKey/SecretKey authentication.
	// if unauthorized, return nil, err
	// if no decision can be made, return nil, [ErrNotProvided]
	// if unexpected error, return nil, "", err
	AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error)
}

// AuthenticateInfo is the verified identity and authentication context
// installed on a request after authentication succeeds.
type AuthenticateInfo struct {
	// Audiences is the set of audiences the authenticator was able to validate
	// the credential against. For an audience-aware authenticator, this is the
	// validated intersection rather than every audience asserted by the
	// credential. If the authenticator is not audience aware, this field is
	// empty.
	Audiences []string
	// User is the canonical principal associated with the credential.
	User UserInfo
}

type SSHAuthenticator interface {
	// BasicAuthenticator is ssh user/password mode authenticator
	BasicAuthenticator
	// AuthenticatePublicKey authenticate in ssh public key mode
	AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error)
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

type AuthenticateErrorHandleFunc func(w http.ResponseWriter, r *http.Request, err error)

func NewAuthenticateFilter(authn Authenticator, onerr AuthenticateErrorHandleFunc) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		r = r.WithContext(WithResponseHeader(r.Context(), w.Header()))
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
