package cache

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// LoadingCache caches immutable values and merges concurrent loads of one key.
// Construct it with NewLoadingCache. Failed loads are never cached. The first
// caller's context controls a shared load; callers share its result and error.
type LoadingCache[T any] struct {
	values Cache[T]
	ttl    time.Duration
	loader func(context.Context, string) (T, error)
	group  singleflight.Group
}

// NewLoadingCache binds a cache backend, nonnegative TTL and loader.
// A zero TTL disables expiration; the backend owns capacity eviction.
func NewLoadingCache[T any](values Cache[T], ttl time.Duration, loader func(context.Context, string) (T, error)) (*LoadingCache[T], error) {
	if ttl < 0 {
		return nil, ErrInvalidTTL
	}
	return &LoadingCache[T]{values: values, ttl: ttl, loader: loader}, nil
}

// GetOrLoad returns a cached value or shares one load for the key. Values, including
// referenced maps and slices, must remain immutable after loading. Each caller
// may cancel its own wait; the first caller controls the shared loader context.
func (c *LoadingCache[T]) GetOrLoad(ctx context.Context, key string) (T, error) {
	value, found, err := c.values.Get(ctx, key)
	if err != nil {
		return value, err
	}
	if found {
		return value, nil
	}
	pending := c.group.DoChan(key, func() (any, error) {
		value, found, err := c.values.Get(ctx, key)
		if err != nil {
			return value, err
		}
		if found {
			return struct{ Value T }{value}, nil
		}
		value, err = c.loader(ctx, key)
		if err != nil {
			return value, err
		}
		if err := c.values.Set(ctx, key, value, c.ttl); err != nil {
			return value, err
		}
		return struct{ Value T }{value}, nil
	})
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case result := <-pending:
		if result.Err != nil {
			var zero T
			return zero, result.Err
		}
		return result.Val.(struct{ Value T }).Value, nil
	}
}

// Set caches an accepted immutable value using the configured TTL. An already
// running load may subsequently replace it; both must represent the same fact.
func (c *LoadingCache[T]) Set(ctx context.Context, key string, value T) error {
	return c.values.Set(ctx, key, value, c.ttl)
}
