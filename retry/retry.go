package retry

import (
	"context"
	"math/rand/v2"
	"time"

	"xiaoshiai.cn/common/log"
)

type Backoff struct {
	// The initial duration.
	Duration time.Duration
	// Duration is multiplied by factor each iteration, if factor is not zero
	// and the limits imposed by Steps and Cap have not been reached.
	// Should not be negative.
	// The jitter does not contribute to the updates to the duration parameter.
	Factor float64
	// The sleep at each iteration is the duration plus an additional
	// amount chosen uniformly at random from the interval between
	// zero and `jitter*duration`.
	Jitter float64
	// A limit on revised values of the duration parameter. If a
	// multiplication by the factor parameter would make the duration
	// exceed the cap then the duration is set to the cap and the
	// steps parameter is set to zero.
	Cap time.Duration
}

// DelayFunc returns the next time interval to wait.
type DelayFunc func() time.Duration

var DefaultBackoff = Backoff{
	Duration: 1 * time.Second,
	Factor:   2,
	Jitter:   0.1,
	Cap:      30 * time.Second,
}

func OnError(ctx context.Context, fn func(ctx context.Context) error) error {
	return BackOff(ctx, DefaultBackoff, fn)
}

func BackOff(ctx context.Context, backoff Backoff, fn func(ctx context.Context) error) error {
	duration, nextwait := backoff.Duration, time.Duration(0)
	for {
		lastoccurred := time.Now()
		err := fn(ctx)
		if err == nil {
			// If the operation was successful, return nil.
			return nil
		}
		// If the last operation took longer than the backoff cap, reset the backoff.
		if lastoccurred.Add(backoff.Cap).Before(time.Now()) {
			duration = backoff.Duration
		}
		nextwait, duration = delay(duration, backoff.Cap, backoff.Factor, backoff.Jitter)
		log.FromContext(ctx).Error(err, "retrying", "in", nextwait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(nextwait):
		}
	}
}

// delay implements the core delay algorithm used in this package.
func delay(duration, cap time.Duration, factor, jitter float64) (wait, next time.Duration) {
	// calculate the next step's interval
	if factor != 0 {
		next = time.Duration(float64(duration) * factor)
		if cap > 0 && next > cap {
			next = cap
		}
	} else {
		next = duration
	}
	// jitter the next step's interval
	if jitter > 0 {
		wait = Jitter(duration, jitter)
	}
	return wait, next
}

func Jitter(duration time.Duration, maxFactor float64) time.Duration {
	return duration + time.Duration(rand.Float64()*min(maxFactor, 1.0)*float64(duration))
}

func Fixed(ctx context.Context, interval time.Duration, fn func(ctx context.Context) error) error {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	tim := time.NewTimer(0)
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		log.FromContext(ctx).Error(err, "retrying fixed", "in", interval)
		select {
		case <-ctx.Done():
			return err
		case <-tim.C:
			tim.Reset(interval)
		}
	}
}
