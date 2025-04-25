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
	return c.cache.Remove(key)
}

func (c LRUCache[T]) Add(key string, value T) {
	c.cache.Add(key, value)
}

func (c LRUCache[T]) Get(key string) (T, bool) {
	return c.cache.Get(key)
}

func (c LRUCache[T]) Len() int {
	return c.cache.Len()
}

func (c LRUCache[T]) Keys() []string {
	return c.cache.Keys()
}
