package inmemory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xiaoshiai.cn/common/eventbus"
	"xiaoshiai.cn/common/eventbus/inmemory"
)

func TestPublishMatchesPatterns(t *testing.T) {
	bus := inmemory.New()
	single := startSubscriber(t, bus, "order.*.v1", "single")
	defer single.stop()
	recursive := startSubscriber(t, bus, "order.**", "recursive")
	defer recursive.stop()

	publish(t, bus, eventbus.Event{Type: "order.created.v1"})
	requireEventType(t, single.events, "order.created.v1")
	requireEventType(t, recursive.events, "order.created.v1")

	publish(t, bus, eventbus.Event{Type: "order.eu.created.v1"})
	requireEventType(t, recursive.events, "order.eu.created.v1")
	select {
	case event := <-single.events:
		t.Fatalf("single-segment subscriber received %q", event.Type)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestNamedConsumerRetainsPendingEventsInMemory(t *testing.T) {
	bus := inmemory.New()
	first := startSubscriber(t, bus, "example", "consumer")
	first.stop()

	publish(t, bus, eventbus.Event{Type: "example"})
	resumed := startSubscriber(t, bus, "example", "consumer")
	defer resumed.stop()
	requireEventType(t, resumed.events, "example")
}

func TestNewConsumerDoesNotReplayEarlierEvents(t *testing.T) {
	bus := inmemory.New()
	publish(t, bus, eventbus.Event{Type: "example"})

	subscriber := startSubscriber(t, bus, "example", "consumer")
	defer subscriber.stop()
	select {
	case event := <-subscriber.events:
		t.Fatalf("new consumer received earlier event %q", event.ID)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestConsumerIDsControlFanout(t *testing.T) {
	t.Run("different IDs each receive the event", func(t *testing.T) {
		bus := inmemory.New()
		first := startSubscriber(t, bus, "example", "first")
		defer first.stop()
		second := startSubscriber(t, bus, "example", "second")
		defer second.stop()

		publish(t, bus, eventbus.Event{Type: "example"})
		requireEventType(t, first.events, "example")
		requireEventType(t, second.events, "example")
	})

	t.Run("empty IDs create independent consumers", func(t *testing.T) {
		bus := inmemory.New()
		first := startSubscriber(t, bus, "example", "")
		defer first.stop()
		second := startSubscriber(t, bus, "example", "")
		defer second.stop()

		publish(t, bus, eventbus.Event{Type: "example"})
		requireEventType(t, first.events, "example")
		requireEventType(t, second.events, "example")
	})

	t.Run("same ID and overlapping patterns consume once", func(t *testing.T) {
		bus := inmemory.New()
		first := startSubscriber(t, bus, "order.*.v1", "shared")
		defer first.stop()
		second := startSubscriber(t, bus, "**", "shared")
		defer second.stop()

		publish(t, bus, eventbus.Event{Type: "order.created.v1"})
		select {
		case <-first.events:
			select {
			case <-second.events:
				t.Fatal("both competing subscribers received the event")
			case <-time.After(20 * time.Millisecond):
			}
		case <-second.events:
			select {
			case <-first.events:
				t.Fatal("both competing subscribers received the event")
			case <-time.After(20 * time.Millisecond):
			}
		case <-time.After(time.Second):
			t.Fatal("neither competing subscriber received the event")
		}
	})

	t.Run("same ID only uses a matching handler", func(t *testing.T) {
		bus := inmemory.New()
		orders := startSubscriber(t, bus, "order.**", "shared")
		defer orders.stop()
		users := startSubscriber(t, bus, "user.**", "shared")
		defer users.stop()

		publish(t, bus, eventbus.Event{Type: "user.created"})
		requireEventType(t, users.events, "user.created")
		select {
		case event := <-orders.events:
			t.Fatalf("non-matching handler received %q", event.Type)
		case <-time.After(20 * time.Millisecond):
		}
	})

	t.Run("same ID does not replay a completed event through another pattern", func(t *testing.T) {
		bus := inmemory.New()
		first := startSubscriber(t, bus, "order.created.v1", "shared")
		publish(t, bus, eventbus.Event{Type: "order.created.v1"})
		requireEventType(t, first.events, "order.created.v1")
		first.stop()

		second := startSubscriber(t, bus, "order.**", "shared")
		defer second.stop()
		select {
		case event := <-second.events:
			t.Fatalf("completed event was delivered again as %q", event.Type)
		case <-time.After(20 * time.Millisecond):
		}
	})
}

func TestPublishGeneratesFieldsCopiesDataAndIsIdempotent(t *testing.T) {
	bus := inmemory.New()
	subscriber := startSubscriber(t, bus, "example", "consumer")
	defer subscriber.stop()
	data := []byte("original")
	metadata := map[string]string{"tenant": "one"}
	event := eventbus.Event{
		Type:     "example",
		Data:     data,
		Metadata: metadata,
	}

	id, err := bus.Publish(t.Context(), event, eventbus.PublishOptions{IdempotencyKey: "operation/1"})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	data[0] = 'X'
	metadata["tenant"] = "changed"
	received := requireEventType(t, subscriber.events, "example")
	if received.ID != id || received.ID == "" || received.Time.IsZero() {
		t.Fatalf("received identity = (%q, %v), want generated ID and time", received.ID, received.Time)
	}
	if string(received.Data) != "original" || received.Metadata["tenant"] != "one" {
		t.Fatalf("received data = (%q, %v), want isolated original values", received.Data, received.Metadata)
	}

	duplicateID, err := bus.Publish(t.Context(), eventbus.Event{
		Type:     "example",
		Data:     []byte("original"),
		Metadata: map[string]string{"tenant": "one"},
	}, eventbus.PublishOptions{IdempotencyKey: "operation/1"})
	if err != nil || duplicateID != id {
		t.Fatalf("duplicate Publish() = (%q, %v), want (%q, nil)", duplicateID, err, id)
	}
	select {
	case <-subscriber.events:
		t.Fatal("idempotent publish delivered the event again")
	case <-time.After(20 * time.Millisecond):
	}

	_, err = bus.Publish(t.Context(), eventbus.Event{
		Type: "example",
		Data: []byte("different"),
	}, eventbus.PublishOptions{IdempotencyKey: "operation/1"})
	if !errors.Is(err, eventbus.ErrConflict) {
		t.Fatalf("conflicting Publish() error = %v, want ErrConflict", err)
	}
}

func TestHandlerRetriesAndCopiesEveryAttempt(t *testing.T) {
	bus := inmemory.New()
	var calls atomic.Int32
	received := make(chan eventbus.Event, 1)
	ready := make(chan eventbus.Event, 1)
	stop := subscribe(t, bus, "example", "consumer", eventbus.HandlerFunc(func(
		_ context.Context,
		event eventbus.Event,
	) error {
		if event.Subject == "subscription-probe" {
			select {
			case ready <- event:
			default:
			}
			return nil
		}
		if calls.Add(1) == 1 {
			event.Data[0] = 'X'
			event.Metadata["attempt"] = "changed"
			return eventbus.RetryAfter(errors.New("temporary"), time.Millisecond)
		}
		received <- event
		return nil
	}), eventbus.SubscribeOptions{MaxAttempts: 2})
	defer stop()
	waitUntilSubscribed(t, bus, "example", ready)

	publish(t, bus, eventbus.Event{
		Type:     "example",
		Data:     []byte("original"),
		Metadata: map[string]string{"attempt": "original"},
	})
	event := requireEventType(t, received, "example")
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}
	if string(event.Data) != "original" || event.Metadata["attempt"] != "original" {
		t.Fatalf("retry received mutated event = (%q, %v)", event.Data, event.Metadata)
	}
}

func TestInvalidPublishAndSubscription(t *testing.T) {
	bus := inmemory.New()
	for _, eventType := range []string{"", "order.*", "order..created"} {
		_, err := bus.Publish(t.Context(), eventbus.Event{Type: eventType}, eventbus.PublishOptions{})
		if !errors.Is(err, eventbus.ErrInvalidArgument) {
			t.Fatalf("Publish(%q) error = %v, want ErrInvalidArgument", eventType, err)
		}
	}
	for _, pattern := range []string{"", "order.**.v1"} {
		err := bus.Subscribe(t.Context(), pattern, eventbus.HandlerFunc(func(
			context.Context,
			eventbus.Event,
		) error {
			return nil
		}), eventbus.SubscribeOptions{})
		if !errors.Is(err, eventbus.ErrInvalidArgument) {
			t.Fatalf("Subscribe(%q) error = %v, want ErrInvalidArgument", pattern, err)
		}
	}
}

type runningSubscriber struct {
	events chan eventbus.Event
	stop   func()
}

func startSubscriber(t *testing.T, bus *inmemory.Bus, pattern string, consumerID string) runningSubscriber {
	t.Helper()
	events := make(chan eventbus.Event, 16)
	ready := make(chan eventbus.Event, 1)
	stop := subscribe(t, bus, pattern, consumerID, eventbus.HandlerFunc(func(
		_ context.Context,
		event eventbus.Event,
	) error {
		if event.Subject == "subscription-probe" {
			select {
			case ready <- event:
			default:
			}
			return nil
		}
		events <- event
		return nil
	}), eventbus.SubscribeOptions{})
	waitUntilSubscribed(t, bus, matchingProbeType(pattern), ready)
	return runningSubscriber{
		events: events,
		stop:   stop,
	}
}

func subscribe(
	t *testing.T,
	bus *inmemory.Bus,
	pattern string,
	consumerID string,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		options.ConsumerID = consumerID
		done <- bus.Subscribe(ctx, pattern, handler, options)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Subscribe() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Error("Subscribe() did not stop")
			}
		})
	}
}

func waitUntilSubscribed(
	t *testing.T,
	bus *inmemory.Bus,
	probeType string,
	events <-chan eventbus.Event,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		publish(t, bus, eventbus.Event{Type: probeType, Subject: "subscription-probe"})
		select {
		case event := <-events:
			if event.Subject == "subscription-probe" {
				return
			}
		case <-time.After(time.Millisecond):
		}
	}
	t.Fatal("subscriber did not become active")
}

func matchingProbeType(pattern string) string {
	switch pattern {
	case "order.*.v1":
		return "order.probe.v1"
	case "order.**":
		return "order.probe"
	case "user.**":
		return "user.probe"
	case "**":
		return "probe"
	default:
		return pattern
	}
}

func publish(t *testing.T, bus *inmemory.Bus, event eventbus.Event) string {
	t.Helper()
	id, err := bus.Publish(t.Context(), event, eventbus.PublishOptions{})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	return id
}

func requireEventType(t *testing.T, events <-chan eventbus.Event, eventType string) eventbus.Event {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != eventType {
			t.Fatalf("event type = %q, want %q", event.Type, eventType)
		}
		return event
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for event %q", eventType)
		return eventbus.Event{}
	}
}
