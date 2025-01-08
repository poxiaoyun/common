package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type TokenAuthenticatorChain []TokenAuthenticator

var _ TokenAuthenticator = TokenAuthenticatorChain{}

func (c TokenAuthenticatorChain) Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.Authenticate(ctx, token)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	return nil, utilerrors.NewAggregate(errlist)
}

type BasicAuthenticatorChain []BasicAuthenticator

var _ BasicAuthenticator = BasicAuthenticatorChain{}

func (c BasicAuthenticatorChain) Authenticate(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	var errlist []error
	for _, authn := range c {
		info, err := authn.Authenticate(ctx, username, password)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		return info, nil
	}
	return nil, utilerrors.NewAggregate(errlist)
}

var _ Authenticator = AuthenticateFunc(nil)

type AuthenticateFunc func(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error)

func (f AuthenticateFunc) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	return f(w, r)
}

func NewAnonymousAuthenticator() *AnonymousAuthenticator {
	return &AnonymousAuthenticator{}
}

type AnonymousAuthenticator struct{}

var _ Authenticator = &AnonymousAuthenticator{}

func (a *AnonymousAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	return &AuthenticateInfo{User: UserInfo{Name: AnonymousUser, Groups: []string{AnonymousUser}}}, nil
}

var _ TokenAuthenticator = &LRUCacheAuthenticator{}

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
