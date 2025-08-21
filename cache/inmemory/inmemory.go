package inmemory

import (
	"context"
	"sync"
	"time"

	"xiaoshiai.cn/common/cache"
)

type Options struct {
	TTLCleanInterval time.Duration
}

var _ cache.Cache[any] = &InMemoryCache[any]{}

func NewTyped[T any](options *Options) *InMemoryCache[T] {
	c := &InMemoryCache[T]{
		namespaces: make(map[string]*namespaceCache[T]),
		stopCh:     make(chan struct{}),
	}
	if options.TTLCleanInterval <= 0 {
		options.TTLCleanInterval = 1 * time.Minute
	}
	go c.startCleaner(options.TTLCleanInterval)
	return c
}

type InMemoryCache[T any] struct {
	mu         sync.RWMutex
	namespaces map[string]*namespaceCache[T]
	stopCh     chan struct{}
}

func (c *InMemoryCache[T]) GetOrLoad(ctx context.Context, key string, loader cache.LoadFunc[T], opts ...cache.GetOrSetOption) (T, error) {
	options := cache.GetOrSetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	if val, ok := nc.get(key); ok {
		return val, nil
	}
	data, ttl, err := loader(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}
	nc.set(key, data, ttl)
	return data, nil
}

func (c *InMemoryCache[T]) Get(ctx context.Context, key string, opts ...cache.GetOption) (T, error) {
	options := cache.GetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	if val, ok := nc.get(key); ok {
		return val, nil
	}
	var zero T
	return zero, nil
}

func (c *InMemoryCache[T]) Set(ctx context.Context, key string, data T, opts ...cache.SetOption) error {
	options := cache.SetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	nc.set(key, data, options.TTL)
	return nil
}

func (c *InMemoryCache[T]) Delete(ctx context.Context, key string, opts ...cache.DeleteOption) error {
	options := cache.DeleteOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	nc.delete(key)
	return nil
}

func (c *InMemoryCache[T]) GetMany(ctx context.Context, keys []string, opts ...cache.GetOption) (map[string]T, error) {
	options := cache.GetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	result := make(map[string]T)
	for _, key := range keys {
		if val, ok := nc.get(key); ok {
			result[key] = val
		}
	}
	return result, nil
}

func (c *InMemoryCache[T]) SetMany(ctx context.Context, items map[string]T, opts ...cache.SetOption) error {
	options := cache.SetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	for k, v := range items {
		nc.set(k, v, options.TTL)
	}
	return nil
}

func (c *InMemoryCache[T]) DeleteMany(ctx context.Context, keys []string, opts ...cache.DeleteOption) error {
	options := cache.DeleteOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	for _, k := range keys {
		nc.delete(k)
	}
	return nil
}

func (c *InMemoryCache[T]) Flush(ctx context.Context, opts ...cache.FlushOption) error {
	options := cache.FlushOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	nc := c.getNS(options.Namespace)
	nc.flush()
	return nil
}

func (c *InMemoryCache[T]) Close() {
	close(c.stopCh)
}

func (c *InMemoryCache[T]) startCleaner(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *InMemoryCache[T]) cleanExpired() {
	c.mu.RLock()
	nss := make([]*namespaceCache[T], 0, len(c.namespaces))
	for _, ns := range c.namespaces {
		nss = append(nss, ns)
	}
	c.mu.RUnlock()
	now := time.Now()
	for _, nc := range nss {
		nc.mu.Lock()
		for k, it := range nc.items {
			if it.expiresAt != (time.Time{}) && now.After(it.expiresAt) {
				delete(nc.items, k)
			}
		}
		nc.mu.Unlock()
	}
}

type item[T any] struct {
	data      T
	expiresAt time.Time
}

type namespaceCache[T any] struct {
	mu    sync.RWMutex
	items map[string]*item[T]
}

func (c *InMemoryCache[T]) getNS(ns string) *namespaceCache[T] {
	c.mu.Lock()
	defer c.mu.Unlock()
	nc, ok := c.namespaces[ns]
	if !ok {
		nc = &namespaceCache[T]{items: make(map[string]*item[T])}
		c.namespaces[ns] = nc
	}
	return nc
}

func (nc *namespaceCache[T]) get(key string) (T, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	it, ok := nc.items[key]
	if !ok || (it.expiresAt != (time.Time{}) && time.Now().After(it.expiresAt)) {
		var zero T
		return zero, false
	}
	return it.data, true
}

func (nc *namespaceCache[T]) set(key string, data T, ttl time.Duration) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	expires := time.Time{}
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	nc.items[key] = &item[T]{data: data, expiresAt: expires}
}

func (nc *namespaceCache[T]) delete(key string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	delete(nc.items, key)
}

func (nc *namespaceCache[T]) flush() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.items = make(map[string]*item[T])
}
