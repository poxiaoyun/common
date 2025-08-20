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

func New(options *Options) cache.Cache {
	c := &InMemoryCache{
		namespaces: make(map[string]*namespaceCache),
		stopCh:     make(chan struct{}),
	}
	if options.TTLCleanInterval <= 0 {
		options.TTLCleanInterval = 1 * time.Minute
	}
	go c.startCleaner(options.TTLCleanInterval)
	return c
}

type InMemoryCache struct {
	mu         sync.RWMutex
	namespaces map[string]*namespaceCache
	stopCh     chan struct{}
}

func (c *InMemoryCache) GetOrSet(ctx context.Context, key string, loader cache.LoadFunc, opts cache.GetOrSetOptions) ([]byte, error) {
	ns := opts.Namespace
	nc := c.getNS(ns)
	if val, ok := nc.get(key); ok {
		return val, nil
	}
	data, err := loader(ctx, key)
	if err != nil {
		return nil, err
	}
	nc.set(key, data, opts.TTL)
	return data, nil
}

func (c *InMemoryCache) Get(ctx context.Context, key string, opts cache.GetOptions) ([]byte, error) {
	nc := c.getNS(opts.Namespace)
	if val, ok := nc.get(key); ok {
		return val, nil
	}
	return nil, nil
}

func (c *InMemoryCache) Set(ctx context.Context, key string, data []byte, opts cache.SetOptions) error {
	nc := c.getNS(opts.Namespace)
	nc.set(key, data, opts.TTL)
	return nil
}

func (c *InMemoryCache) Delete(ctx context.Context, key string, opts cache.DeleteOptions) error {
	nc := c.getNS(opts.Namespace)
	nc.delete(key)
	return nil
}

func (c *InMemoryCache) GetMany(ctx context.Context, keys []string, opts cache.GetOptions) (map[string][]byte, error) {
	nc := c.getNS(opts.Namespace)
	result := make(map[string][]byte)
	for _, key := range keys {
		if val, ok := nc.get(key); ok {
			result[key] = val
		}
	}
	return result, nil
}

func (c *InMemoryCache) SetMany(ctx context.Context, items map[string][]byte, opts cache.SetOptions) error {
	nc := c.getNS(opts.Namespace)
	for k, v := range items {
		nc.set(k, v, opts.TTL)
	}
	return nil
}

func (c *InMemoryCache) DeleteMany(ctx context.Context, keys []string, opts cache.DeleteOptions) error {
	nc := c.getNS(opts.Namespace)
	for _, k := range keys {
		nc.delete(k)
	}
	return nil
}

func (c *InMemoryCache) Flush(ctx context.Context, opts cache.FlushOptions) error {
	nc := c.getNS(opts.Namespace)
	nc.flush()
	return nil
}

func (c *InMemoryCache) Close() {
	close(c.stopCh)
}

func (c *InMemoryCache) startCleaner(interval time.Duration) {
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

func (c *InMemoryCache) cleanExpired() {
	c.mu.RLock()
	nss := make([]*namespaceCache, 0, len(c.namespaces))
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

type item struct {
	data      []byte
	expiresAt time.Time
}

type namespaceCache struct {
	mu    sync.RWMutex
	items map[string]*item
}

func (c *InMemoryCache) getNS(ns string) *namespaceCache {
	c.mu.Lock()
	defer c.mu.Unlock()
	nc, ok := c.namespaces[ns]
	if !ok {
		nc = &namespaceCache{items: make(map[string]*item)}
		c.namespaces[ns] = nc
	}
	return nc
}

func (nc *namespaceCache) get(key string) ([]byte, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	it, ok := nc.items[key]
	if !ok || (it.expiresAt != (time.Time{}) && time.Now().After(it.expiresAt)) {
		return nil, false
	}
	return it.data, true
}

func (nc *namespaceCache) set(key string, data []byte, ttl time.Duration) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	expires := time.Time{}
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	nc.items[key] = &item{data: data, expiresAt: expires}
}

func (nc *namespaceCache) delete(key string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	delete(nc.items, key)
}

func (nc *namespaceCache) flush() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.items = make(map[string]*item)
}
