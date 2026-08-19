package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaoshiai.cn/common/cache"
)

func TestFixedWindowLimiterDecisions(t *testing.T) {
	resetAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	counter := &counterStub{expiresAt: resetAt}
	limiter, err := cache.NewFixedWindowLimiter(counter, 2)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}

	wants := []cache.Decision{
		{Allowed: true, Remaining: 1, ResetAt: resetAt},
		{Allowed: true, Remaining: 0, ResetAt: resetAt},
		{Allowed: false, Remaining: 0, ResetAt: resetAt},
	}
	for index, want := range wants {
		got, err := limiter.Allow(t.Context(), "subject")
		if err != nil {
			t.Fatalf("allow attempt %d: %v", index+1, err)
		}
		if got != want {
			t.Fatalf("attempt %d decision = %#v, want %#v", index+1, got, want)
		}
	}

	if counter.key != "subject" {
		t.Fatalf("counter key = %q, want subject", counter.key)
	}
	if counter.value != 3 {
		t.Fatalf("counter value = %d, want rejected attempt to produce 3", counter.value)
	}
}

func TestFixedWindowLimiterReturnsCounterError(t *testing.T) {
	counterErr := errors.New("counter unavailable")
	limiter, err := cache.NewFixedWindowLimiter(&counterStub{err: counterErr}, 1)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}

	decision, err := limiter.Allow(t.Context(), "subject")
	if !errors.Is(err, counterErr) {
		t.Fatalf("allow error = %v, want counter error", err)
	}
	if decision != (cache.Decision{}) {
		t.Fatalf("error decision = %#v, want zero value", decision)
	}
}

func TestNewFixedWindowLimiterValidatesPolicy(t *testing.T) {
	tests := []struct {
		name    string
		limit   int64
		wantErr error
	}{
		{name: "zero limit", limit: 0, wantErr: cache.ErrInvalidLimit},
		{name: "negative limit", limit: -1, wantErr: cache.ErrInvalidLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := cache.NewFixedWindowLimiter(&counterStub{}, test.limit)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("constructor error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

type counterStub struct {
	value     int64
	expiresAt time.Time
	key       string
	err       error
}

func (c *counterStub) Add(ctx context.Context, key string, amount int64) (cache.Count, error) {
	if c.err != nil {
		return cache.Count{}, c.err
	}
	c.value += amount
	c.key = key
	return cache.Count{Value: c.value, ExpiresAt: c.expiresAt}, nil
}
