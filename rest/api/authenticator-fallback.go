package api

import (
	"errors"
	"net/http"
)

// FallbackAuthenticator authenticates with fallback only when primary does not apply.
type FallbackAuthenticator struct {
	primary  Authenticator
	fallback Authenticator
}

// NewFallbackAuthenticator composes primary with an authenticator used when primary returns ErrNotProvided.
func NewFallbackAuthenticator(primary, fallback Authenticator) *FallbackAuthenticator {
	return &FallbackAuthenticator{primary: primary, fallback: fallback}
}

// Authenticate returns the primary result unless primary returns ErrNotProvided.
func (a *FallbackAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
	info, err := a.primary.Authenticate(w, r)
	if !errors.Is(err, ErrNotProvided) {
		return info, err
	}
	return a.fallback.Authenticate(w, r)
}

var _ Authenticator = (*FallbackAuthenticator)(nil)
