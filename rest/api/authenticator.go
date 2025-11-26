package api

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/errors"
)

type TokenAuthenticatorChain []TokenAuthenticator

var _ TokenAuthenticator = TokenAuthenticatorChain{}

func (c TokenAuthenticatorChain) AuthenticateToken(ctx context.Context, token string) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateToken(ctx, token)
		if err != nil {
			if err == ErrNotProvided {
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

func (c BasicAuthenticatorChain) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateBasic(ctx, username, password)
		if err != nil {
			if err == ErrNotProvided {
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

func (c SSHAuthenticatorChain) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticateBasic(ctx, username, password)
		if err != nil {
			if err == ErrNotProvided {
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

func (c SSHAuthenticatorChain) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.AuthenticatePublicKey(ctx, pubkey)
		if err != nil {
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

var _ Authenticator = AuthenticateFunc(nil)

func SessionAuthenticatorWrap(authn TokenAuthenticator, sessionkey string) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtractTokenFromCookie(r, sessionkey)
		if token == "" {
			return nil, ErrNotProvided
		}
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authn.AuthenticateToken(ctx, token)
	})
}

func ExtractTokenFromCookie(r *http.Request, cookieName string) string {
	cookie, _ := r.Cookie(cookieName)
	if cookie != nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

func BearerTokenAuthenticatorWrap(authn TokenAuthenticator) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		token := ExtractBearerTokenFromRequest(r)
		if token == "" {
			return nil, ErrNotProvided
		}
		ctx := WithResponseHeader(r.Context(), w.Header())
		return authn.AuthenticateToken(ctx, token)
	})
}

func ExtractBearerTokenFromRequest(r *http.Request) string {
	token := r.Header.Get("Authorization")
	// only support bearer token
	if after, ok := strings.CutPrefix(token, "Bearer "); ok {
		return after
	}
	return r.URL.Query().Get("token")
}

func BasicAuthenticatorWrap(authn BasicAuthenticator) Authenticator {
	return AuthenticateFunc(func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
		username, password, ok := r.BasicAuth()
		if !ok {
			return nil, ErrNotProvided
		}
		return authn.AuthenticateBasic(r.Context(), username, password)
	})
}

type AuthenticatorChain []Authenticator

func (d AuthenticatorChain) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	var errs []error
	for _, a := range d {
		info, err := a.Authenticate(w, r)
		if err != nil {
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

type AuthenticateFunc func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error)

func (f AuthenticateFunc) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	return f(w, r)
}

