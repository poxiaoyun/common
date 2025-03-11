package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"xiaoshiai.cn/common/log"
)

// DefaultRetry is the recommended retry for a conflict where multiple clients
// are making changes to the same resource.
var DefaultRetry = wait.Backoff{
	Steps:    5,
	Duration: 1 * time.Second,
	Factor:   1.0,
	Jitter:   0.1,
}

var AlwaysRetry = func(err error) bool { return true }

// OnError allows the caller to retry fn in case the error returned by fn is retriable
// according to the provided function. backoff defines the maximum retries and the wait
// interval between two retries.
func RetryOnError(ctx context.Context, backoff wait.Backoff, retriable func(error) bool, fn func(ctx context.Context) error) error {
	var lastErr error
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		err := fn(ctx)
		switch {
		case err == nil:
			return true, nil
		case retriable(err):
			lastErr = err
			return false, nil
		default:
			return false, err
		}
	})
	if wait.Interrupted(err) {
		err = lastErr
	}
	return err
}

func RetryFixIntervalContext(ctx context.Context, interval time.Duration, fn func(ctx context.Context) error) error {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	tim := time.NewTimer(0)
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		log.FromContext(ctx).Error(err, "retrying", "in", interval)
		select {
		case <-ctx.Done():
			return err
		case <-tim.C:
			tim.Reset(interval)
		}
	}
}

func RetryContext(ctx context.Context, fn func(ctx context.Context) error) error {
	return RetryOnError(ctx, DefaultRetry, AlwaysRetry, fn)
}
