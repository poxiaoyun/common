package eventbus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"xiaoshiai.cn/common/pattern"
)

var (
	// ErrInvalidArgument is returned when an event, subscription, handler, or
	// option is invalid.
	ErrInvalidArgument = errors.New("invalid eventbus argument")

	// ErrConflict is returned when an event ID or idempotency key is reused for
	// a different event.
	ErrConflict = errors.New("eventbus resource conflicts with an existing resource")
)

// Event is an immutable fact delivered through the bus.
//
// Type identifies the event schema and is required. It must be an exact,
// dot-separated event type without wildcard tokens. Versioning should be part
// of Type, for example "order.created.v1". ID may be empty when publishing, in
// which case the adapter generates one. Time may be zero, in which case the
// adapter records its current time. Publish and Subscribe must not retain or
// mutate caller-owned Data or Metadata.
type Event struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Source   string            `json:"source,omitempty"`
	Subject  string            `json:"subject,omitempty"`
	Time     time.Time         `json:"time"`
	Data     []byte            `json:"data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PublishOptions controls how an event is accepted. Its zero value requests a
// normal publish.
type PublishOptions struct {
	// IdempotencyKey makes retries of an equivalent publish return the original
	// event ID instead of creating another event. Reusing the key for a different
	// event must return ErrConflict.
	IdempotencyKey string
}

// Publisher accepts events for asynchronous delivery.
//
// A successful return means the adapter accepted responsibility for the event;
// it does not wait for handlers. Cancellation of ctx after a successful return
// does not cancel the event. The returned ID is the accepted event's ID.
type Publisher interface {
	Publish(ctx context.Context, event Event, options PublishOptions) (string, error)
}

// Handler handles one delivery attempt. Implementations may call it more than
// once for the same event. A nil error acknowledges the event. A regular error
// requests a retry according to SubscribeOptions; use NoRetry or RetryAfter to
// override that decision.
type Handler interface {
	Handle(ctx context.Context, event Event) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, event Event) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// SubscribeOptions defines delivery policy. Zero values select adapter
// defaults.
type SubscribeOptions struct {
	// ConsumerID identifies consumption progress across all subscriptions using
	// that ID. Concurrent Subscribe calls with the same non-empty ID compete for
	// each event matched by any of their patterns. An empty ID creates a distinct
	// ephemeral consumer for every Subscribe call.
	ConsumerID string

	MaxConcurrency int
	MaxAttempts    int
	Timeout        time.Duration
}

// Subscriber consumes events asynchronously.
//
// Subscribe blocks until ctx is canceled or the adapter encounters a fatal
// infrastructure error. Handler errors are delivery outcomes and must not make
// Subscribe return. A newly created consumer starts with events accepted after
// it becomes active; an existing durable consumer resumes its pending progress.
// Subscribers with different consumer IDs each receive an event independently.
// Subscribe calls with the same non-empty consumer ID share one consumer and
// compete for each event. Their patterns only select which handlers are
// eligible: when several patterns of one consumer match the same event, one of
// the matching handlers receives it. eventTypePattern uses dot-separated
// segments: a literal matches exactly, "*" matches one segment, and a final
// "**" matches zero or more trailing segments. For example, "order.foo*"
// matches "order.foobar", and "order.**" matches both "order" and
// "order.created.v1". A "**" segment is only valid as the final segment.
//
// ConsumerID alone is the logical consumption identity. Patterns do not create
// independent progress or cause duplicate delivery. Once a consumer completes
// an event, another subscription using that ConsumerID must not receive the
// event again even if its pattern also matches. A pattern also applies to event
// types first published after Subscribe starts.
type Subscriber interface {
	Subscribe(ctx context.Context, eventTypePattern string, handler Handler, options SubscribeOptions) error
}

// ValidateEventType validates an exact dot-separated event type. For example,
// "order.created.v1" is valid, while "order.*" and "order..created" are not.
func ValidateEventType(value string) error {
	if invalidEventTypeSections(value) || strings.ContainsAny(value, "*,{}") {
		return fmt.Errorf("%w: invalid event type %q", ErrInvalidArgument, value)
	}
	return nil
}

// CompileEventTypePattern compiles the dot-separated pattern accepted by
// Subscriber implementations. For example, "order.*.v1" selects one event
// type level and "order.**" selects order plus all of its descendants.
func CompileEventTypePattern(value string) (pattern.Wildcard, error) {
	doubleStar := strings.Index(value, "**")
	if invalidEventTypeSections(value) ||
		doubleStar >= 0 && (doubleStar != len(value)-2 || doubleStar > 0 && value[doubleStar-1] != '.') {
		return pattern.Wildcard{}, fmt.Errorf("%w: invalid event type pattern %q", ErrInvalidArgument, value)
	}
	compiled, err := pattern.CompileWildcard(value, pattern.WildcardOptions{Separator: '.'})
	if err != nil {
		return pattern.Wildcard{}, fmt.Errorf("%w: event type pattern: %v", ErrInvalidArgument, err)
	}
	return compiled, nil
}

func invalidEventTypeSections(value string) bool {
	return value == "" || value[0] == '.' || value[len(value)-1] == '.' || strings.Contains(value, "..")
}

// NoRetry marks err as a permanent delivery failure. A nil error remains nil.
func NoRetry(err error) error {
	if err == nil {
		return nil
	}
	return &HandlerError{
		Err:     err,
		NoRetry: true,
	}
}

// IsNoRetry reports whether err or an error it wraps was marked by NoRetry.
func IsNoRetry(err error) bool {
	var target *HandlerError
	return errors.As(err, &target) && target.NoRetry
}

// RetryAfter requests another delivery attempt after at least delay. A nil
// error remains nil. NoRetry takes precedence. A non-positive delay leaves err
// unchanged and selects the adapter's default retry policy.
func RetryAfter(err error, delay time.Duration) error {
	if err == nil {
		return nil
	}
	if delay <= 0 || IsNoRetry(err) {
		return err
	}
	return &HandlerError{
		Err:        err,
		RetryAfter: delay,
	}
}

// RetryDelay returns the delay requested by RetryAfter. The boolean is false
// when err has no explicit retry delay.
func RetryDelay(err error) (time.Duration, bool) {
	var target *HandlerError
	if !errors.As(err, &target) || target.NoRetry || target.RetryAfter <= 0 {
		return 0, false
	}
	return target.RetryAfter, true
}

// HandlerError describes an explicit retry decision returned by a Handler.
// Values should normally be created with NoRetry or RetryAfter.
type HandlerError struct {
	Err        error
	NoRetry    bool
	RetryAfter time.Duration
}

func (e *HandlerError) Error() string {
	if e == nil || e.Err == nil {
		return "event handler failed"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying Handler error.
func (e *HandlerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
