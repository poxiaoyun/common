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
	TTL       time.Duration
}

type LoadFunc func(ctx context.Context, key string) ([]byte, error)

type Cache interface {
	GetOrSet(ctx context.Context, key string, loader LoadFunc, opts GetOrSetOptions) ([]byte, error)
	Get(ctx context.Context, key string, opts GetOptions) ([]byte, error)
	Set(ctx context.Context, key string, data []byte, opts SetOptions) error
	Delete(ctx context.Context, key string, opts DeleteOptions) error

	GetMany(ctx context.Context, keys []string, opts GetOptions) (map[string][]byte, error)
	SetMany(ctx context.Context, items map[string][]byte, opts SetOptions) error
	DeleteMany(ctx context.Context, keys []string, opts DeleteOptions) error

	Flush(ctx context.Context, opts FlushOptions) error
}
