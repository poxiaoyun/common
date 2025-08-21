package cache

import (
	"context"
	"time"
)

type SetOptions struct {
	TTL       time.Duration
	Namespace string
}
type GetOptions struct {
	Namespace string
}

type DeleteOptions struct {
	Namespace string
}

type FlushOptions struct {
	Namespace string
}

type GetOrSetOptions struct {
	Namespace string
}

type LoadFunc[T any] func(ctx context.Context, key string) (T, time.Duration, error)

type (
	GetOrSetOption func(opts *GetOrSetOptions)
	GetOption      func(opts *GetOptions)
	SetOption      func(opts *SetOptions)
	DeleteOption   func(opts *DeleteOptions)
	FlushOption    func(opts *FlushOptions)
)

type Cache[T any] interface {
	GetOrLoad(ctx context.Context, key string, loader LoadFunc[T], opts ...GetOrSetOption) (T, error)
	Get(ctx context.Context, key string, opts ...GetOption) (T, error)
	Set(ctx context.Context, key string, data T, opts ...SetOption) error
	Delete(ctx context.Context, key string, opts ...DeleteOption) error

	GetMany(ctx context.Context, keys []string, opts ...GetOption) (map[string]T, error)
	SetMany(ctx context.Context, items map[string]T, opts ...SetOption) error
	DeleteMany(ctx context.Context, keys []string, opts ...DeleteOption) error

	Flush(ctx context.Context, opts ...FlushOption) error
}
