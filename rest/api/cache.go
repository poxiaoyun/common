package api

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

func NewLRUCache[T any](size int, ttl time.Duration) LRUCache[T] {
	return LRUCache[T]{cache: expirable.NewLRU[string, T](size, nil, ttl)}
}

type LRUCache[T any] struct {
	cache *expirable.LRU[string, T]
}

func (c LRUCache[T]) GetOrAdd(key string, fn func() (T, error)) (T, error) {
	if c.cache == nil {
		return fn()
	}
	if info, ok := c.cache.Get(key); ok {
		return info, nil
	}
	info, err := fn()
	if err != nil {
		return info, err
	}
	c.cache.Add(key, info)
	return info, nil
}

func (c LRUCache[T]) Remove(key string) bool {
	if c.cache == nil {
		return false
	}
	return c.cache.Remove(key)
}
