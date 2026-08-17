package oidc

import (
	"context"
	"sync"
)

// CacheState describes whether a cached result can be returned and whether it
// should be refreshed.
type CacheState uint8

const (
	// CacheFresh returns the cached result without refreshing it.
	CacheFresh CacheState = iota
	// CacheStale returns the cached result and refreshes it in the background.
	CacheStale
	// CacheExpired waits for a refreshed result instead of returning the cache.
	CacheExpired
)

// SingleFlight runs at most one operation at a time and retains its most
// recent successful result.
type SingleFlight[T any] struct {
	mu      sync.Mutex
	current *singleFlightCall[T]
	last    T
	hasLast bool
}

type singleFlightCall[T any] struct {
	done   chan struct{}
	result T
	err    error
}

// Get returns the last successful result according to its state. A stale
// result is returned while one background operation refreshes it; an expired
// result waits for the current or newly started operation.
func (f *SingleFlight[T]) Get(ctx context.Context, state func(T) CacheState, operation func(context.Context) (T, error)) (T, error) {
	f.mu.Lock()
	if f.hasLast {
		switch state(f.last) {
		case CacheFresh:
			result := f.last
			f.mu.Unlock()
			return result, nil
		case CacheStale:
			result := f.last
			f.startLocked(context.WithoutCancel(ctx), operation)
			f.mu.Unlock()
			return result, nil
		}
	}
	current := f.startLocked(ctx, operation)
	f.mu.Unlock()
	return waitSingleFlight(ctx, current)
}

// Refresh runs operation regardless of the cached result, or waits for the
// operation already in progress.
func (f *SingleFlight[T]) Refresh(ctx context.Context, operation func(context.Context) (T, error)) (T, error) {
	f.mu.Lock()
	current := f.startLocked(ctx, operation)
	f.mu.Unlock()
	return waitSingleFlight(ctx, current)
}

func (f *SingleFlight[T]) startLocked(ctx context.Context, operation func(context.Context) (T, error)) *singleFlightCall[T] {
	current := f.current
	if current != nil {
		return current
	}
	current = &singleFlightCall[T]{done: make(chan struct{})}
	f.current = current
	go f.run(ctx, current, operation)
	return current
}

func (f *SingleFlight[T]) run(ctx context.Context, current *singleFlightCall[T], operation func(context.Context) (T, error)) {
	result, err := operation(ctx)
	f.mu.Lock()
	if err != nil {
		current.err = err
		f.current = nil
		close(current.done)
		f.mu.Unlock()
		return
	}
	f.last = result
	f.hasLast = true
	current.result = result
	f.current = nil
	close(current.done)
	f.mu.Unlock()
}

func waitSingleFlight[T any](ctx context.Context, current *singleFlightCall[T]) (T, error) {
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case <-current.done:
		return current.result, current.err
	}
}
