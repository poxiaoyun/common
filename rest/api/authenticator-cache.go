package api

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

func NewCacheTokenAuthenticator(authenticator TokenAuthenticator, size int, ttl time.Duration) *LRUCacheAuthenticator {
	return &LRUCacheAuthenticator{
		Authenticator: authenticator,
		Cache:         NewLRUCache[*AuthenticateInfo](size, ttl),
	}
}

type LRUCacheAuthenticator struct {
	Authenticator TokenAuthenticator
	Cache         LRUCache[*AuthenticateInfo]
}

// Authenticate implements TokenAuthenticator.
func (a *LRUCacheAuthenticator) Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error) {
	// do not cache anonymous user
	if token == "" {
		return a.Authenticator.Authenticate(ctx, token)
	}
	return a.Cache.GetOrAdd(token, func() (*AuthenticateInfo, error) {
		return a.Authenticator.Authenticate(ctx, token)
	})
}

func NewCachedSSHAuthenticator(authenticator SSHAuthenticator, size int, ttl time.Duration) *LRUCacheSSHAuthenticator {
	return &LRUCacheSSHAuthenticator{Authenticator: authenticator, Cache: NewLRUCache[*AuthenticateInfo](size, ttl)}
}

var _ SSHAuthenticator = &LRUCacheSSHAuthenticator{}

type LRUCacheSSHAuthenticator struct {
	Authenticator SSHAuthenticator
	Cache         LRUCache[*AuthenticateInfo]
}

// AuthenticatePublibcKey implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) AuthenticatePublibcKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error) {
	return a.Cache.GetOrAdd(ssh.FingerprintSHA256(pubkey), func() (*AuthenticateInfo, error) {
		return a.Authenticator.AuthenticatePublibcKey(ctx, pubkey)
	},
	)
}

// AuthenticatePassword implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) Authenticate(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return a.Cache.GetOrAdd(fmt.Sprintf("%s:%s", username, password), func() (*AuthenticateInfo, error) {
		return a.Authenticator.Authenticate(ctx, username, password)
	})
}
