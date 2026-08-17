package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"maps"
	"slices"
)

// StaticTokenAuthenticator maps one opaque token to one fixed user identity.
// It retains only a digest of the credential and copies returned identity values.
type StaticTokenAuthenticator struct {
	// Digest is the SHA-256 digest compared with presented credentials.
	Digest [sha256.Size]byte
	// User is the identity returned for a matching credential.
	User UserInfo
}

// NewStaticTokenAuthenticator creates an authenticator for one opaque token and fixed user identity.
func NewStaticTokenAuthenticator(token string, user UserInfo) *StaticTokenAuthenticator {
	return &StaticTokenAuthenticator{
		Digest: sha256.Sum256([]byte(token)),
		User:   cloneUserInfo(user),
	}
}

// AuthenticateToken returns the configured identity when token matches.
func (a *StaticTokenAuthenticator) AuthenticateToken(_ context.Context, token string) (*AuthenticateInfo, error) {
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(a.Digest[:], digest[:]) != 1 {
		return nil, ErrNotProvided
	}
	return &AuthenticateInfo{User: cloneUserInfo(a.User)}, nil
}

func cloneUserInfo(user UserInfo) UserInfo {
	user.Groups = slices.Clone(user.Groups)
	user.Extra = maps.Clone(user.Extra)
	for key, values := range user.Extra {
		user.Extra[key] = slices.Clone(values)
	}
	return user
}

var _ TokenAuthenticator = &StaticTokenAuthenticator{}
