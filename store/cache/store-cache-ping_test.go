package cache

import (
	"context"
	"testing"

	"xiaoshiai.cn/common/store"
)

type pingStore struct {
	store.Store
	called bool
}

func (s *pingStore) Ping(context.Context) error {
	s.called = true
	return nil
}

func TestCacheStorePing(t *testing.T) {
	underlying := &pingStore{}
	storage := NewCacheStore(underlying)

	if err := storage.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if !underlying.called {
		t.Fatal("Ping() was not delegated to the underlying store")
	}
}
