// Package eventbus defines contracts for publishing and subscribing to
// asynchronous events.
//
// Events are broadcast between distinct consumer IDs. ConsumerID alone owns
// consumption progress. Concurrent subscribers using the same non-empty ID
// share one logical consumer and compete for events matched by any of their
// patterns. Overlapping patterns do not duplicate an event for that consumer.
// When ConsumerID is empty, every Subscribe call creates a distinct ephemeral
// consumer, so each such subscriber receives its own copy.
//
// Publishers use exact, dot-separated event types. Subscribers may select an
// exact type, use "*" for one segment, or end a pattern with "**" for zero or
// more trailing segments. Pattern matching applies only to Event.Type; payload
// and metadata filtering are outside this package's contract.
//
// Delivery is asynchronous with respect to Publish. A successful Publish means
// that the adapter accepted responsibility for the event; it does not mean that
// any Handler has run. Durable adapters should provide at-least-once delivery,
// so handlers must make their side effects idempotent. An in-memory adapter may
// lose accepted events when the process exits and should document that
// limitation.
package eventbus
