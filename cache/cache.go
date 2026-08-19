// Package cache defines backend-independent capabilities for expiring values
// and counters.
package cache

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidTTL indicates that an operation received a TTL outside its
	// supported range.
	ErrInvalidTTL = errors.New("cache: invalid TTL")
	// ErrInvalidWindow indicates that a windowed capability was configured
	// without a positive window.
	ErrInvalidWindow = errors.New("cache: window must be positive")
	// ErrInvalidCapacity indicates that a budget was configured without a
	// positive capacity.
	ErrInvalidCapacity = errors.New("cache: capacity must be positive")
	// ErrInvalidAmount indicates that a counter operation received a
	// non-positive amount.
	ErrInvalidAmount = errors.New("cache: amount must be positive")
	// ErrCounterOverflow indicates that an addition cannot be represented by
	// the counter value type.
	ErrCounterOverflow = errors.New("cache: counter overflow")
)

// Cache stores best-effort expiring values. Implementations may evict values
// before their TTL, so callers must remain correct on every cache miss.
//
// Values are immutable across this interface. Callers must not mutate a value
// after passing it to Set or mutate a value returned by Get.
type Cache[T any] interface {
	// Get returns the current value for key. A miss returns the zero value,
	// false, and a nil error.
	Get(ctx context.Context, key string) (value T, found bool, err error)

	// Set replaces the value for key. A zero TTL means the value does not
	// expire; a negative TTL returns ErrInvalidTTL.
	Set(ctx context.Context, key string, value T, ttl time.Duration) error

	// Delete removes key. Deleting a missing key succeeds.
	Delete(ctx context.Context, key string) error
}

// WindowedAccumulator atomically accumulates positive weighted amounts during
// a configured window. Unlike Cache values, live state must not be evicted
// before its expiration.
type WindowedAccumulator interface {
	// Add creates a missing or expired counter at amount, or adds amount to the
	// current value. Creation fixes the expiration using the window bound to the
	// adapter; later additions do not extend it.
	Add(ctx context.Context, key string, amount int64) (Count, error)
}

// Count is the state of a windowed accumulator after an addition.
type Count struct {
	// Value is the accumulated value.
	Value int64
	// ExpiresAt is the authoritative expiration time chosen by the adapter.
	ExpiresAt time.Time
}

// Consumption is the result of consuming a windowed budget.
type Consumption struct {
	// Granted reports whether the amount was consumed.
	Granted bool
	// Used is the amount consumed in the current window.
	Used int64
	// Remaining is the amount still available in the current window.
	Remaining int64
	// ExpiresAt is the current window's authoritative expiration time. It is
	// zero when a rejected operation did not create a window.
	ExpiresAt time.Time
}

// WindowedBudget atomically consumes positive weighted amounts without
// exceeding the capacity bound to the adapter. Unlike Cache values, live state
// must not be evicted before its expiration.
type WindowedBudget interface {
	// TryConsume consumes amount only when the resulting usage does not exceed
	// the configured capacity. A rejected operation does not create or change
	// state. The configured window is fixed when state is first created and is
	// not extended by later operations.
	TryConsume(ctx context.Context, key string, amount int64) (Consumption, error)
}
