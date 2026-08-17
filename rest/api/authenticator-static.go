package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
)

// StaticTokenAuthenticator maps one opaque token to fixed authentication info.
// It retains only a digest of the credential and copies returned values.
type StaticTokenAuthenticator struct {
	// Digest is the SHA-256 digest compared with presented credentials.
	Digest [sha256.Size]byte
	// Authentication is returned for a matching credential.
	Authentication AuthenticationInfo
}

// NewStaticTokenAuthenticator creates an authenticator for one opaque token and
// fixed authentication info.
func NewStaticTokenAuthenticator(token string, authentication AuthenticationInfo) *StaticTokenAuthenticator {
	return &StaticTokenAuthenticator{
		Digest:         sha256.Sum256([]byte(token)),
		Authentication: authentication.Clone(),
	}
}

// AuthenticateToken returns the configured identity when token matches.
func (a *StaticTokenAuthenticator) AuthenticateToken(_ context.Context, token string) (*AuthenticationInfo, error) {
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(a.Digest[:], digest[:]) != 1 {
		return nil, ErrNotProvided
	}
	info := a.Authentication.Clone()
	return &info, nil
}

var _ TokenAuthenticator = &StaticTokenAuthenticator{}
