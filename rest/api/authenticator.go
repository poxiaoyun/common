package api

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/errors"
)

type TokenAuthenticatorChain []TokenAuthenticator

var _ TokenAuthenticator = TokenAuthenticatorChain{}

func (c TokenAuthenticatorChain) AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateToken(ctx, token)
		if err != nil {
			if stderrors.Is(err, ErrNotProvided) {
				continue
			}
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	if len(errlist) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errlist)
}

type BasicAuthenticatorChain []BasicAuthenticator

var _ BasicAuthenticator = BasicAuthenticatorChain{}

func (c BasicAuthenticatorChain) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateBasic(ctx, username, password)
		if err != nil {
			if stderrors.Is(err, ErrNotProvided) {
				continue
			}
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	if len(errlist) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errlist)
}

type SSHAuthenticatorChain []SSHAuthenticator

var _ SSHAuthenticator = SSHAuthenticatorChain{}

func (c SSHAuthenticatorChain) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateBasic(ctx, username, password)
		if err != nil {
			if stderrors.Is(err, ErrNotProvided) {
				continue
			}
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	if len(errlist) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errlist)
}

func (c SSHAuthenticatorChain) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticationInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticatePublicKey(ctx, pubkey)
		if err != nil {
			if stderrors.Is(err, ErrNotProvided) {
				continue
			}
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	if len(errlist) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errlist)
}

var _ Authenticator = AuthenticatorFunc(nil)

// NewSessionAuthenticator adapts token authentication to a session cookie.
func NewSessionAuthenticator(authenticator TokenAuthenticator, sessionKey string) Authenticator {
	return AuthenticatorFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
		token := ExtractTokenFromCookie(r, sessionKey)
		if token == "" {
			return nil, ErrNotProvided
		}
		info, err := authenticator.AuthenticateToken(r.Context(), token)
		if stderrors.Is(err, ErrNotProvided) {
			return nil, errors.NewUnauthorized("session credential was not accepted")
		}
		return info, err
	})
}

func ExtractTokenFromCookie(r *http.Request, cookieName string) string {
	cookie, _ := r.Cookie(cookieName)
	if cookie != nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// NewBearerTokenAuthenticator adapts token authentication to HTTP Bearer
// credentials.
func NewBearerTokenAuthenticator(authenticator TokenAuthenticator) Authenticator {
	return AuthenticatorFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
		token, provided := extractBearerTokenFromRequest(r)
		if !provided {
			return nil, ErrNotProvided
		}
		if token == "" {
			return nil, errors.NewUnauthorized("bearer token is empty")
		}
		info, err := authenticator.AuthenticateToken(r.Context(), token)
		if stderrors.Is(err, ErrNotProvided) {
			return nil, errors.NewUnauthorized("bearer token was not accepted")
		}
		return info, err
	})
}

// BearerTokenAuthenticationError writes the Bearer challenge carried by err,
// or a bare Bearer challenge when err has no more specific challenge.
func BearerTokenAuthenticationError(w http.ResponseWriter, _ *http.Request, err error) {
	challenge := "Bearer"
	var challengeErr *AuthenticationChallengeError
	if stderrors.As(err, &challengeErr) {
		Error(w, err)
		return
	}
	w.Header().Set("WWW-Authenticate", challenge)
	Unauthorized(w, "Unauthorized")
}

func ExtractBearerTokenFromRequest(r *http.Request) string {
	token, _ := extractBearerTokenFromRequest(r)
	return token
}

func extractBearerTokenFromRequest(r *http.Request) (string, bool) {
	authorization := r.Header.Get("Authorization")
	scheme, token, hasValue := strings.Cut(authorization, " ")
	if strings.EqualFold(scheme, "Bearer") {
		if !hasValue {
			return "", true
		}
		return token, true
	}
	values, provided := r.URL.Query()["token"]
	if !provided {
		return "", false
	}
	return values[0], true
}

// NewBasicAuthenticator adapts basic authentication to HTTP Basic
// credentials.
func NewBasicAuthenticator(authenticator BasicAuthenticator) Authenticator {
	return AuthenticatorFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
		username, password, ok := r.BasicAuth()
		if !ok {
			return nil, ErrNotProvided
		}
		info, err := authenticator.AuthenticateBasic(r.Context(), username, password)
		if stderrors.Is(err, ErrNotProvided) {
			return nil, errors.NewUnauthorized("basic credentials were not accepted")
		}
		return info, err
	})
}

type AuthenticatorChain []Authenticator

func (d AuthenticatorChain) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
	var errs []error
	for _, a := range d {
		info, err := a.Authenticate(w, r)
		if err != nil {
			if stderrors.Is(err, ErrNotProvided) {
				continue
			}
			errs = append(errs, err)
			continue
		}
		return info, nil
	}
	if len(errs) == 0 {
		return nil, ErrNotProvided
	}
	return nil, errors.NewAggregate(errs)
}

// AuthenticatorFunc adapts a function to Authenticator.
type AuthenticatorFunc func(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error)

// Authenticate calls f.
func (f AuthenticatorFunc) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
	return f(w, r)
}
