package authn

import (
	"context"
	"time"

	"xiaoshiai.cn/common/rest/api"
)

type CacheOptions struct {
	ProfileSize int           `json:"profileSize,omitempty"`
	ProfileTime time.Duration `json:"profileTime,omitempty"`
	APIKeySize  int           `json:"apiKeySize,omitempty"`
	APIKeyTime  time.Duration `json:"apiKeyTime,omitempty"`
}

func NewDefaultCacheOptions() *CacheOptions {
	return &CacheOptions{
		ProfileSize: 32,
		APIKeySize:  32,
		ProfileTime: 5 * time.Minute,
		APIKeyTime:  5 * time.Minute,
	}
}

func NewLRUProviderCache(option *CacheOptions, authp Provider) *LRUProviderCache {
	return &LRUProviderCache{
		ProfileCache: api.NewLRUCache[UserProfile](option.ProfileSize, option.ProfileTime),
		APIKeyCache:  api.NewLRUCache[User](option.APIKeySize, option.APIKeyTime),
		Provider:     authp,
	}
}

var _ Provider = &LRUProviderCache{}

type LRUProviderCache struct {
	ProfileCache api.LRUCache[UserProfile]
	APIKeyCache  api.LRUCache[User]
	Provider
}

func (c *LRUProviderCache) GetCurrentProfile(ctx context.Context, session string) (*UserProfile, error) {
	profile, err := c.ProfileCache.GetOrAdd(session, func() (UserProfile, error) {
		ptr, err := c.Provider.GetCurrentProfile(ctx, session)
		if err != nil {
			return UserProfile{}, err
		}
		return *ptr, err
	})
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (c *LRUProviderCache) UpdateCurrentProfile(ctx context.Context, session string, data UserProfile) error {
	c.ProfileCache.Remove(session)
	return c.Provider.UpdateCurrentProfile(ctx, session, data)
}

func (c *LRUProviderCache) UpdateUserProfile(ctx context.Context, profile *UserProfile) error {
	for _, key := range c.ProfileCache.Keys() {
		val, ok := c.ProfileCache.Get(key)
		if !ok {
			continue
		}
		if val.Name == profile.Name {
			c.ProfileCache.Remove(key)
		}
	}
	return c.Provider.UpdateUserProfile(ctx, profile)
}

func (c *LRUProviderCache) CheckAPIKey(ctx context.Context, key APIKey) (*User, error) {
	user, err := c.APIKeyCache.GetOrAdd(key.AccessKey, func() (User, error) {
		ptr, err := c.Provider.CheckAPIKey(ctx, key)
		if err != nil {
			return User{}, err
		}
		return *ptr, err
	})
	return &user, err
}

func (c *LRUProviderCache) Signout(ctx context.Context, session string) error {
	c.ProfileCache.Remove(session)
	return c.Provider.Signout(ctx, session)
}
