// Package inmemory provides an ephemeral event bus for tests, local
// development, and process-local module decoupling. Events and consumer
// progress are lost when the process exits.
package inmemory

import (
	"bytes"
	"context"
	"maps"
	"sync"
	"time"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/eventbus"
	"xiaoshiai.cn/common/pattern"
)

const (
	defaultMaxConcurrency = 1
	defaultMaxAttempts    = 3
)

// New creates an empty in-memory event bus.
func New() *Bus {
	return &Bus{
		events:      make(map[string]eventbus.Event),
		idempotency: make(map[string]string),
		consumers:   make(map[consumerKey]*consumer),
	}
}

var (
	_ eventbus.Publisher  = (*Bus)(nil)
	_ eventbus.Subscriber = (*Bus)(nil)
)

// Bus publishes events to process-local consumers. A named consumer retains
// its consumption progress while it has no active Subscribe call, but that
// state remains in memory only. An unnamed consumer exists only for the
// duration of its Subscribe call. Events published before a logical consumer
// is first created are not replayed. Queues are unbounded, so Publish does not
// apply backpressure.
type Bus struct {
	mu            sync.Mutex
	events        map[string]eventbus.Event
	eventOrder    []string
	idempotency   map[string]string
	consumers     map[consumerKey]*consumer
	nextEphemeral uint64
}

type consumerKey struct {
	id        string
	ephemeral uint64
}

type consumer struct {
	activatedAt   int
	registrations map[*registration]struct{}
	deliveries    map[string]*delivery
	pending       []*delivery
}

type registration struct {
	pattern pattern.Wildcard
	wakeup  chan struct{}
}

type delivery struct {
	event    eventbus.Event
	attempts int
}

// Publish accepts an event and queues one delivery for every matching logical
// consumer. Repeating an accepted event ID or IdempotencyKey with an equivalent
// event returns the original ID without delivering the event again.
func (b *Bus) Publish(
	ctx context.Context,
	event eventbus.Event,
	options eventbus.PublishOptions,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := eventbus.ValidateEventType(event.Type); err != nil {
		return "", err
	}
	event = cloneEvent(event)

	b.mu.Lock()
	defer b.mu.Unlock()
	if options.IdempotencyKey != "" {
		if id, exists := b.idempotency[options.IdempotencyKey]; exists {
			if equivalentEvent(b.events[id], event) {
				return id, nil
			}
			return "", eventbus.ErrConflict
		}
	}
	if event.ID != "" {
		if existing, exists := b.events[event.ID]; exists {
			if !equivalentEvent(existing, event) {
				return "", eventbus.ErrConflict
			}
			if options.IdempotencyKey != "" {
				b.idempotency[options.IdempotencyKey] = event.ID
			}
			return event.ID, nil
		}
	} else {
		event.ID = uuid.NewString()
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	b.events[event.ID] = event
	b.eventOrder = append(b.eventOrder, event.ID)
	if options.IdempotencyKey != "" {
		b.idempotency[options.IdempotencyKey] = event.ID
	}
	for _, group := range b.consumers {
		b.enqueue(group, event)
	}
	return event.ID, nil
}

// Subscribe consumes events matching eventTypePattern until ctx is canceled.
// Its wildcard and consumer identity semantics are defined by
// eventbus.Subscriber. Handler failures are retried immediately unless they use
// eventbus.RetryAfter; MaxAttempts includes the first delivery. Zero
// MaxConcurrency and MaxAttempts values default to one and three respectively.
func (b *Bus) Subscribe(
	ctx context.Context,
	eventTypePattern string,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	compiled, err := eventbus.CompileEventTypePattern(eventTypePattern)
	if err != nil {
		return err
	}
	if handler == nil || options.MaxConcurrency < 0 || options.MaxAttempts < 0 || options.Timeout < 0 {
		return eventbus.ErrInvalidArgument
	}
	if options.MaxConcurrency == 0 {
		options.MaxConcurrency = defaultMaxConcurrency
	}
	if options.MaxAttempts == 0 {
		options.MaxAttempts = defaultMaxAttempts
	}

	b.mu.Lock()
	key := consumerKey{
		id: options.ConsumerID,
	}
	if options.ConsumerID == "" {
		b.nextEphemeral++
		key.ephemeral = b.nextEphemeral
	}
	group, exists := b.consumers[key]
	if !exists {
		group = &consumer{
			activatedAt:   len(b.eventOrder),
			registrations: make(map[*registration]struct{}),
			deliveries:    make(map[string]*delivery),
		}
		b.consumers[key] = group
	}
	registered := &registration{
		pattern: compiled,
		wakeup:  make(chan struct{}, 1),
	}
	group.registrations[registered] = struct{}{}
	for _, id := range b.eventOrder[group.activatedAt:] {
		event := b.events[id]
		if registered.pattern.Match(event.Type) {
			b.enqueue(group, event)
		}
	}
	for _, item := range group.pending {
		if registered.pattern.Match(item.event.Type) {
			notify(registered)
			break
		}
	}
	b.mu.Unlock()

	var workers sync.WaitGroup
	workers.Add(options.MaxConcurrency)
	for range options.MaxConcurrency {
		go func() {
			defer workers.Done()
			b.consume(ctx, group, registered, handler, options)
		}()
	}
	workers.Wait()

	b.mu.Lock()
	delete(group.registrations, registered)
	if key.ephemeral != 0 {
		delete(b.consumers, key)
	}
	b.mu.Unlock()
	return nil
}

func (b *Bus) consume(
	ctx context.Context,
	group *consumer,
	registered *registration,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) {
	for {
		item, ok := b.take(ctx, group, registered)
		if !ok {
			return
		}
		if b.deliver(ctx, item, handler, options) {
			b.mu.Lock()
			group.pending = append(group.pending, item)
			b.notifyMatching(group, item.event)
			b.mu.Unlock()
			return
		}
	}
}

func (b *Bus) take(
	ctx context.Context,
	group *consumer,
	registered *registration,
) (*delivery, bool) {
	for {
		b.mu.Lock()
		for index, item := range group.pending {
			if !registered.pattern.Match(item.event.Type) {
				continue
			}
			group.pending = append(group.pending[:index], group.pending[index+1:]...)
			b.notifyMatching(group, item.event)
			b.mu.Unlock()
			return item, true
		}
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, false
		case <-registered.wakeup:
		}
	}
}

// deliver returns true when cancellation interrupted a delivery or its retry
// delay and the event must remain pending for another Subscribe call.
func (b *Bus) deliver(
	ctx context.Context,
	item *delivery,
	handler eventbus.Handler,
	options eventbus.SubscribeOptions,
) bool {
	for {
		item.attempts++
		handlerContext := ctx
		cancel := func() {}
		if options.Timeout > 0 {
			handlerContext, cancel = context.WithTimeout(ctx, options.Timeout)
		}
		err := handler.Handle(handlerContext, cloneEvent(item.event))
		cancel()
		if err == nil {
			return false
		}
		if ctx.Err() != nil {
			item.attempts--
			return true
		}
		if eventbus.IsNoRetry(err) || item.attempts >= options.MaxAttempts {
			return false
		}
		delay, delayed := eventbus.RetryDelay(err)
		if !delayed {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return true
		case <-timer.C:
		}
	}
}

func (b *Bus) enqueue(group *consumer, event eventbus.Event) {
	if _, exists := group.deliveries[event.ID]; exists {
		return
	}
	matched := false
	for registered := range group.registrations {
		if registered.pattern.Match(event.Type) {
			matched = true
			break
		}
	}
	if !matched {
		return
	}
	item := &delivery{event: event}
	group.deliveries[event.ID] = item
	group.pending = append(group.pending, item)
	b.notifyMatching(group, event)
}

func (b *Bus) notifyMatching(group *consumer, event eventbus.Event) {
	for registered := range group.registrations {
		if registered.pattern.Match(event.Type) {
			notify(registered)
		}
	}
}

func notify(registered *registration) {
	select {
	case registered.wakeup <- struct{}{}:
	default:
	}
}

func cloneEvent(event eventbus.Event) eventbus.Event {
	event.Data = bytes.Clone(event.Data)
	event.Metadata = maps.Clone(event.Metadata)
	return event
}

func equivalentEvent(existing eventbus.Event, candidate eventbus.Event) bool {
	return (candidate.ID == "" || candidate.ID == existing.ID) &&
		candidate.Type == existing.Type &&
		candidate.Source == existing.Source &&
		candidate.Subject == existing.Subject &&
		(candidate.Time.IsZero() || candidate.Time.Equal(existing.Time)) &&
		bytes.Equal(candidate.Data, existing.Data) &&
		maps.Equal(candidate.Metadata, existing.Metadata)
}
