package oidc

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestSingleFlightCombinesConcurrentCalls(t *testing.T) {
	var flight SingleFlight[int]
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	operation := func(context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		close(finished)
		return 42, nil
	}

	result := make(chan int)
	go func() {
		value, err := flight.Refresh(context.Background(), operation)
		if err != nil {
			t.Errorf("Do: %v", err)
		}
		result <- value
	}()
	<-started
	for range 10 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := flight.Refresh(ctx, operation)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("operation calls = %d", calls.Load())
	}
	close(release)
	<-finished
	if value := <-result; value != 42 {
		t.Fatalf("result = %d", value)
	}
}

func TestSingleFlightDoCanStopWaiting(t *testing.T) {
	var flight SingleFlight[int]
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	operation := func(context.Context) (int, error) {
		close(started)
		<-release
		close(finished)
		return 42, nil
	}

	go func() {
		_, _ = flight.Refresh(context.Background(), operation)
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := flight.Refresh(ctx, operation)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	close(release)
	<-finished
}

func TestSingleFlightGetReturnsStaleAndStartsOnlyOneRefresh(t *testing.T) {
	var flight SingleFlight[int]
	if _, err := flight.Refresh(context.Background(), func(context.Context) (int, error) {
		return 41, nil
	}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	operation := func(context.Context) (int, error) {
		calls.Add(1)
		close(started)
		<-release
		close(finished)
		return 42, nil
	}

	result, err := flight.Get(context.Background(), func(int) CacheState {
		return CacheStale
	}, operation)
	if err != nil {
		t.Fatal(err)
	}
	if result != 41 {
		t.Fatalf("stale result = %d", result)
	}
	<-started
	result, err = flight.Get(context.Background(), func(int) CacheState {
		return CacheStale
	}, operation)
	if err != nil {
		t.Fatal(err)
	}
	if result != 41 {
		t.Fatalf("stale result = %d", result)
	}
	close(release)
	<-finished
	if calls.Load() != 1 {
		t.Fatalf("operation calls = %d", calls.Load())
	}
}

func TestSingleFlightGetWaitsWhenExpired(t *testing.T) {
	var flight SingleFlight[int]
	if _, err := flight.Refresh(context.Background(), func(context.Context) (int, error) {
		return 41, nil
	}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan int)
	go func() {
		value, err := flight.Get(context.Background(), func(int) CacheState {
			return CacheExpired
		}, func(context.Context) (int, error) {
			close(started)
			<-release
			return 42, nil
		})
		if err != nil {
			t.Errorf("Get: %v", err)
		}
		result <- value
	}()
	<-started
	select {
	case value := <-result:
		t.Fatalf("Get returned expired result %d", value)
	default:
	}
	close(release)
	if value := <-result; value != 42 {
		t.Fatalf("result = %d", value)
	}
}

func TestSingleFlightFailureRetainsLastSuccess(t *testing.T) {
	var flight SingleFlight[int]
	if _, err := flight.Refresh(context.Background(), func(context.Context) (int, error) {
		return 42, nil
	}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("failed")
	if _, err := flight.Refresh(context.Background(), func(context.Context) (int, error) {
		return 0, want
	}); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	result, err := flight.Get(context.Background(), func(int) CacheState {
		return CacheFresh
	}, func(context.Context) (int, error) {
		return 0, errors.New("unexpected load")
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != 42 {
		t.Fatalf("last success after failure = %d", result)
	}
}

func TestSingleFlightRetriesAfterFailure(t *testing.T) {
	var flight SingleFlight[int]
	want := errors.New("failed")
	if _, err := flight.Refresh(context.Background(), func(context.Context) (int, error) {
		return 0, want
	}); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	result, err := flight.Get(context.Background(), func(int) CacheState { return CacheFresh }, func(context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != 42 {
		t.Fatalf("result = %d", result)
	}
}
