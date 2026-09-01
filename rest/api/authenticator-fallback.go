package api

import (
	"errors"
	"net/http"
)

// FallbackAuthenticator authenticates with fallback only when primary does not apply.
type FallbackAuthenticator struct {
	primary  HTTPAuthenticator
	fallback HTTPAuthenticator
}

// NewFallbackAuthenticator composes primary with an authenticator used when primary returns ErrNotProvided.
func NewFallbackAuthenticator(primary, fallback HTTPAuthenticator) *FallbackAuthenticator {
	return &FallbackAuthenticator{primary: primary, fallback: fallback}
}

// AuthenticateHTTP returns the primary result unless primary returns ErrNotProvided.
func (a *FallbackAuthenticator) AuthenticateHTTP(w http.ResponseWriter, r *http.Request) (*Authentication, error) {
	info, err := a.primary.AuthenticateHTTP(w, r)
	if !errors.Is(err, ErrNotProvided) {
		return info, err
	}
	return a.fallback.AuthenticateHTTP(w, r)
}

var _ HTTPAuthenticator = (*FallbackAuthenticator)(nil)
