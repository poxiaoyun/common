// Package sequence allocates monotonic numbers within caller-defined keys.
package sequence

import "context"

// Allocator allocates numbers independently for each stable key. Allocated
// numbers are unique and increasing, but gaps are allowed.
type Allocator interface {
	// Next returns the next positive number for key.
	Next(ctx context.Context, key string) (uint64, error)
}
