package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
)

// StaticTokenAuthenticator maps one opaque token to fixed authentication.
// It retains only a digest of the credential and copies returned values.
type StaticTokenAuthenticator struct {
	// Digest is the SHA-256 digest compared with presented credentials.
	Digest [sha256.Size]byte
	// Authentication is returned for a matching credential.
	Authentication Authentication
}

// NewStaticTokenAuthenticator creates an authenticator for one opaque token and
// fixed authentication.
func NewStaticTokenAuthenticator(token string, authentication Authentication) *StaticTokenAuthenticator {
	return &StaticTokenAuthenticator{
		Digest:         sha256.Sum256([]byte(token)),
		Authentication: copyAuthentication(authentication),
	}
}

// AuthenticateToken returns the configured identity when token matches.
func (a *StaticTokenAuthenticator) AuthenticateToken(_ context.Context, token string) (*Authentication, error) {
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(a.Digest[:], digest[:]) != 1 {
		return nil, ErrNotProvided
	}
	info := copyAuthentication(a.Authentication)
	return &info, nil
}

var _ TokenAuthenticator = &StaticTokenAuthenticator{}
