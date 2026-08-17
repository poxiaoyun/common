// Package inmemory provides a process-local sequence allocator.
package inmemory

import (
	"context"
	"sync"
)

// Allocator allocates sequences in process memory.
type Allocator struct {
	mu     sync.Mutex
	values map[string]uint64
}

// New creates an empty in-memory allocator.
func New() *Allocator {
	return &Allocator{values: map[string]uint64{}}
}

// Next implements sequence.Allocator.
func (a *Allocator) Next(_ context.Context, key string) (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.values[key]++
	return a.values[key], nil
}
