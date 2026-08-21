package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/crypto/ssh"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

// ErrNotProvided means an authenticator did not claim its input. At the request
// seam it means the request contained no applicable credential. Credential
// adapters may use it to let another adapter inspect the same credential, but
// must translate chain exhaustion into an authentication error before returning
// to the request seam.
var ErrNotProvided = fmt.Errorf("no authentication provided")

const AnonymousSubjectID = "anonymous"

// AuthenticationChallengeError carries an HTTP authentication challenge,
// response status from an authentication or authorization adapter to the final
// response filter.
type AuthenticationChallengeError struct {
	// Status is the public error response written when the challenge is final.
	*commonerrors.Status
	// Challenge is the complete WWW-Authenticate field value.
	Challenge string
}

// NewUnauthorizedChallengeError returns a challenged HTTP 401 error with a
// public response message.
func NewUnauthorizedChallengeError(challenge, message string) *AuthenticationChallengeError {
	return &AuthenticationChallengeError{
		Status:    commonerrors.NewUnauthorized(message),
		Challenge: challenge,
	}
}

// NewForbiddenChallengeError returns a challenged HTTP 403 error with a public
// response message.
func NewForbiddenChallengeError(challenge, message string) *AuthenticationChallengeError {
	return &AuthenticationChallengeError{
		Status:    commonerrors.NewCustomError(http.StatusForbidden, commonerrors.StatusReasonForbidden, message),
		Challenge: challenge,
	}
}

// Unwrap exposes the public response status to errors.Is and errors.As.
func (err *AuthenticationChallengeError) Unwrap() error {
	return err.Status
}

// Subject is the identity a request is about. It may represent a person,
// application, workload, or another authenticated principal.
type Subject struct {
	// ID is the stable identifier used by authorization, ownership, and audit.
	ID string `json:"id"`
	// Name is the provider-verified username or principal name within the
	// authentication domain. It is not a stable ownership key.
	Name string `json:"name,omitempty"`
	// DisplayName is a human-facing, non-unique label.
	DisplayName string `json:"displayName,omitempty"`
	// Email is the authenticated subject email when the authentication method
	// provides one.
	Email string `json:"email,omitempty"`
	// EmailVerified reports whether the authentication provider verified Email.
	EmailVerified bool `json:"emailVerified,omitempty"`
	// Groups contains authorization groups assigned to the subject.
	Groups []string `json:"groups,omitempty"`
}

// AccessConstraints contains authorization constraints carried by an OAuth
// 2.0 access token. A non-nil value identifies an access-token authentication,
// including when Scopes is empty.
type AccessConstraints struct {
	Audiences []string `json:"audiences,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
}

type Authenticator interface {
	// Authenticate authenticates the request and returns the authentication info.
	// if the request has no applicable credential, return nil, [ErrNotProvided]
	// so that another authenticator or fallback can run. A rejected request that
	// requires an HTTP challenge returns [AuthenticationChallengeError].
	// A returned [commonerrors.Status] is intentionally public response data;
	// any other error is diagnostic and receives the default response.
	// once authenticated, return the AuthenticationInfo, nil
	Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error)
}

type TokenAuthenticator interface {
	// AuthenticateToken authenticates the token and returns the authentication info.
	// if unauthorized, return nil, err
	// if no decision can be made, return nil, [ErrNotProvided]
	// if unexpected error, return nil, "", err
	AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error)
}

type BasicAuthenticator interface {
	// AuthenticateBasic authenticates the username and password and returns the authentication info.
	// It also use for APIKey/SecretKey authentication.
	// if unauthorized, return nil, err
	// if no decision can be made, return nil, [ErrNotProvided]
	// if unexpected error, return nil, "", err
	AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error)
}

// AuthenticationInfo is the verified identity and authentication context
// installed on a request after authentication succeeds. Subject is embedded so
// the simplest authentication result is the subject itself.
type AuthenticationInfo struct {
	Subject
	// Actor is the current subject acting on behalf of Subject.
	Actor *Subject `json:"actor,omitempty"`
	// Access contains constraints from an OAuth 2.0 access token.
	Access *AccessConstraints `json:"access,omitempty"`
}

// Clone returns an independent snapshot for implementations that retain
// authentication across requests, such as static authenticators and caches.
// Request-local context, audit, and transport propagation do not need it.
func (info AuthenticationInfo) Clone() AuthenticationInfo {
	cloned := info
	cloned.Groups = append([]string(nil), info.Groups...)
	if info.Actor != nil {
		actor := *info.Actor
		actor.Groups = append([]string(nil), info.Actor.Groups...)
		cloned.Actor = &actor
	}
	if info.Access != nil {
		cloned.Access = &AccessConstraints{
			Audiences: append([]string(nil), info.Access.Audiences...),
			Scopes:    append([]string(nil), info.Access.Scopes...),
		}
	}
	return cloned
}

// ValidateAuthenticationInfo validates the canonical authentication value at a
// transport seam.
func ValidateAuthenticationInfo(info AuthenticationInfo) error {
	if info.ID == "" {
		return fmt.Errorf("subject ID is required")
	}
	if info.Actor != nil && info.Actor.ID == "" {
		return fmt.Errorf("actor ID is required")
	}
	return nil
}

type SSHAuthenticator interface {
	// BasicAuthenticator is ssh user/password mode authenticator
	BasicAuthenticator
	// AuthenticatePublicKey authenticate in ssh public key mode
	AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticationInfo, error)
}

// WithAuthentication returns a context carrying authentication.
func WithAuthentication(ctx context.Context, info AuthenticationInfo) context.Context {
	return SetContextValue(ctx, "authentication", info)
}

// AuthenticationFromContext returns the authentication carried by ctx.
func AuthenticationFromContext(ctx context.Context) AuthenticationInfo {
	return GetContextValue[AuthenticationInfo](ctx, "authentication")
}

// WithAuthenticationAudiences returns a context carrying the audiences a
// token authenticator is expected to validate.
func WithAuthenticationAudiences(ctx context.Context, audiences []string) context.Context {
	return SetContextValue(ctx, "authentication-audiences", audiences)
}

// AuthenticationAudiencesFromContext returns the expected token audiences.
func AuthenticationAudiencesFromContext(ctx context.Context) []string {
	return GetContextValue[[]string](ctx, "authentication-audiences")
}

// NewBearerTokenAuthenticationFilter authenticates Bearer credentials and
// returns RFC 6750 challenges when authentication fails.
func NewBearerTokenAuthenticationFilter(authenticator TokenAuthenticator) Filter {
	return NewAuthenticationFilter(
		NewBearerTokenAuthenticator(authenticator),
		BearerTokenAuthenticationError,
	)
}

// AuthenticationErrorHandlerFunc owns logging and response redaction for a
// failed authentication attempt.
type AuthenticationErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)

// NewAuthenticationFilter authenticates requests and installs successful
// authentication in the request context.
func NewAuthenticationFilter(authenticator Authenticator, onError AuthenticationErrorHandlerFunc) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		info, err := authenticator.Authenticate(w, r)
		if err != nil {
			if onError != nil {
				onError(w, r, err)
				return
			}
			var status *commonerrors.Status
			if errors.As(err, &status) {
				// A Status is an intentional public response from the authenticator.
				Error(w, err)
			} else {
				// Keep diagnostic details in logs and return the default response.
				log.FromContext(r.Context()).Error(err, "authentication failed")
				Unauthorized(w, "Unauthorized")
			}
			return
		}
		if err := ValidateAuthenticationInfo(*info); err != nil {
			ServerError(w, fmt.Errorf("authenticator returned invalid authentication info: %w", err))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAuthentication(r.Context(), *info)))
	})
}
