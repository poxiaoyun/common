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
		Cache:         NewLRUCache[*AuthenticationInfo](size, ttl),
	}
}

type LRUCacheAuthenticator struct {
	Authenticator TokenAuthenticator
	Cache         LRUCache[*AuthenticationInfo]
}

// AuthenticateToken implements TokenAuthenticator.
func (a *LRUCacheAuthenticator) AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error) {
	// do not cache anonymous user
	if token == "" {
		return a.Authenticator.AuthenticateToken(ctx, token)
	}
	info, err := a.Cache.GetOrAdd(token, func() (*AuthenticationInfo, error) {
		return a.Authenticator.AuthenticateToken(ctx, token)
	})
	if err != nil {
		return nil, err
	}
	cloned := info.Clone()
	return &cloned, nil
}

func NewCachedSSHAuthenticator(authenticator SSHAuthenticator, size int, ttl time.Duration) *LRUCacheSSHAuthenticator {
	return &LRUCacheSSHAuthenticator{Authenticator: authenticator, Cache: NewLRUCache[*AuthenticationInfo](size, ttl)}
}

var _ SSHAuthenticator = &LRUCacheSSHAuthenticator{}

type LRUCacheSSHAuthenticator struct {
	Authenticator SSHAuthenticator
	Cache         LRUCache[*AuthenticationInfo]
}

// AuthenticatePublicKey implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticationInfo, error) {
	info, err := a.Cache.GetOrAdd(ssh.FingerprintSHA256(pubkey), func() (*AuthenticationInfo, error) {
		return a.Authenticator.AuthenticatePublicKey(ctx, pubkey)
	})
	if err != nil {
		return nil, err
	}
	cloned := info.Clone()
	return &cloned, nil
}

// AuthenticatePassword implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	info, err := a.Cache.GetOrAdd(fmt.Sprintf("%s:%s", username, password), func() (*AuthenticationInfo, error) {
		return a.Authenticator.AuthenticateBasic(ctx, username, password)
	})
	if err != nil {
		return nil, err
	}
	cloned := info.Clone()
	return &cloned, nil
}
