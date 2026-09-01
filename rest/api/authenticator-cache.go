package api

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"

	"golang.org/x/crypto/ssh"
)

func NewCacheTokenAuthenticator(authenticator TokenAuthenticator, size int, ttl time.Duration) *LRUCacheAuthenticator {
	return &LRUCacheAuthenticator{
		Authenticator: authenticator,
		Cache:         NewLRUCache[*Authentication](size, ttl),
	}
}

type LRUCacheAuthenticator struct {
	Authenticator TokenAuthenticator
	Cache         LRUCache[*Authentication]
}

// AuthenticateToken implements TokenAuthenticator.
func (a *LRUCacheAuthenticator) AuthenticateToken(ctx context.Context, token string) (*Authentication, error) {
	// do not cache anonymous user
	if token == "" {
		return a.Authenticator.AuthenticateToken(ctx, token)
	}
	info, err := a.Cache.GetOrAdd(authenticationCacheKey(token), func() (*Authentication, error) {
		return a.Authenticator.AuthenticateToken(ctx, token)
	})
	if err != nil {
		return nil, err
	}
	result := copyAuthentication(*info)
	return &result, nil
}

func NewCachedSSHAuthenticator(authenticator SSHAuthenticator, size int, ttl time.Duration) *LRUCacheSSHAuthenticator {
	return &LRUCacheSSHAuthenticator{Authenticator: authenticator, Cache: NewLRUCache[*Authentication](size, ttl)}
}

var _ SSHAuthenticator = &LRUCacheSSHAuthenticator{}

type LRUCacheSSHAuthenticator struct {
	Authenticator SSHAuthenticator
	Cache         LRUCache[*Authentication]
}

// AuthenticateSSHPublicKey implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) AuthenticateSSHPublicKey(ctx context.Context, username string, publicKey ssh.PublicKey) (*Authentication, error) {
	key := authenticationCacheKey(username, ssh.FingerprintSHA256(publicKey))
	info, err := a.Cache.GetOrAdd(key, func() (*Authentication, error) {
		return a.Authenticator.AuthenticateSSHPublicKey(ctx, username, publicKey)
	})
	if err != nil {
		return nil, err
	}
	result := copyAuthentication(*info)
	return &result, nil
}

// AuthenticateBasic implements SSHAuthenticator.
func (a *LRUCacheSSHAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*Authentication, error) {
	info, err := a.Cache.GetOrAdd(authenticationCacheKey(username, password), func() (*Authentication, error) {
		return a.Authenticator.AuthenticateBasic(ctx, username, password)
	})
	if err != nil {
		return nil, err
	}
	result := copyAuthentication(*info)
	return &result, nil
}

func authenticationCacheKey(parts ...string) string {
	digest := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(part))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func copyAuthentication(authentication Authentication) Authentication {
	result := authentication
	result.Subject.Groups = append([]string(nil), authentication.Subject.Groups...)
	if authentication.Actor != nil {
		actor := *authentication.Actor
		actor.Groups = append([]string(nil), authentication.Actor.Groups...)
		result.Actor = &actor
	}
	if authentication.Token != nil {
		result.Token = &TokenInfo{
			Audiences: append([]string(nil), authentication.Token.Audiences...),
			Scopes:    append([]string(nil), authentication.Token.Scopes...),
		}
	}
	return result
}
