package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/authn"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

// ErrNotProvided means an authenticator did not claim its input. At the request
// seam it means the request contained no applicable credential. Credential
// adapters may use it to let another adapter inspect the same credential, but
// must translate chain exhaustion into an authentication error before returning
// to the request seam.
var ErrNotProvided = errors.New("authentication not provided")

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

// Subject aliases the canonical authenticated subject owned by authn.
type Subject = authn.Subject

// Authentication aliases the canonical verified authentication result owned
// by authn.
type Authentication = authn.Authentication

// TokenInfo aliases canonical verified access-token metadata owned by authn.
type TokenInfo = authn.TokenInfo

// HTTPAuthenticator authenticates credentials carried by an HTTP request.
type HTTPAuthenticator interface {
	// AuthenticateHTTP authenticates credentials carried by r. If r has no
	// applicable credential, it returns nil, [ErrNotProvided]
	// so that another authenticator or fallback can run. A rejected request that
	// requires an HTTP challenge returns [AuthenticationChallengeError].
	// A returned [commonerrors.Status] is intentionally public response data;
	// any other error is diagnostic and receives the default response.
	// Successful authentication returns authn.Authentication and a nil error.
	AuthenticateHTTP(w http.ResponseWriter, r *http.Request) (*authn.Authentication, error)
}

// TokenAuthenticator authenticates token credentials accepted by HTTP
// Bearer, cookie, or protocol-specific request adapters.
type TokenAuthenticator interface {
	// AuthenticateToken authenticates token and returns the REST request
	// authentication. ErrNotProvided lets another configured token adapter try
	// the same credential.
	AuthenticateToken(ctx context.Context, token string) (*authn.Authentication, error)
}

// BasicAuthenticator authenticates an HTTP Basic username/password or API key
// pair.
type BasicAuthenticator interface {
	// AuthenticateBasic authenticates the HTTP Basic credential. ErrNotProvided
	// lets another configured Basic adapter try the same credential.
	AuthenticateBasic(ctx context.Context, username, password string) (*authn.Authentication, error)
}

// SSHAuthenticator authenticates SSH password and public-key credentials used
// by REST-adjacent SSH adapters in this package.
type SSHAuthenticator interface {
	BasicAuthenticator
	// AuthenticateSSHPublicKey authenticates the SSH username and public-key
	// credential after the protocol has verified private-key possession.
	AuthenticateSSHPublicKey(ctx context.Context, username string, publicKey ssh.PublicKey) (*authn.Authentication, error)
}

// ValidateAuthentication validates authentication received at an external
// seam.
func ValidateAuthentication(authentication authn.Authentication) error {
	if authentication.ID == "" {
		return fmt.Errorf("subject ID is required")
	}
	if authentication.Actor != nil && authentication.Actor.ID == "" {
		return fmt.Errorf("actor ID is required")
	}
	return nil
}

// WithAuthentication returns a context carrying authentication.
func WithAuthentication(ctx context.Context, authentication authn.Authentication) context.Context {
	return SetContextValue(ctx, "authentication", authentication)
}

// AuthenticationFromContext returns the authentication carried by ctx.
func AuthenticationFromContext(ctx context.Context) authn.Authentication {
	return GetContextValue[authn.Authentication](ctx, "authentication")
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
func NewAuthenticationFilter(authenticator HTTPAuthenticator, onError AuthenticationErrorHandlerFunc) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		info, err := authenticator.AuthenticateHTTP(w, r)
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
		if err := ValidateAuthentication(*info); err != nil {
			ServerError(w, fmt.Errorf("authenticator returned invalid authentication info: %w", err))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAuthentication(r.Context(), *info)))
	})
}
