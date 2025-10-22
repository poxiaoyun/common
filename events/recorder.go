package events

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/queue"
	"xiaoshiai.cn/common/store"
)

type Reason string

const (
	Week = 7 * 24 * time.Hour
	Day  = 24 * time.Hour
)

type Event struct {
	// UID is the unique identifier of every event
	UID string `json:"uid,omitempty"`
	// Identifier is the identifier of the event, it is used to aggregate the event
	Identifier        string                         `json:"identifier,omitempty"`
	Annotations       map[string]string              `json:"annotations,omitempty"`
	Object            store.ResourcedObjectReference `json:"object,omitempty"`
	Reason            Reason                         `json:"reason,omitempty"`
	Message           string                         `json:"message,omitempty"`
	CreationTimestamp meta.Time                      `json:"creationTimestamp,omitempty"`
}

type Recorder interface {
	// Event records an event
	// it aggregate the event with same reason and object
	// the same event will be recorded only once
	Event(ctx context.Context, obj store.Object, reason Reason, message string) error
	EventAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error

	// EventNoAggregate records an event without aggregate
	EventNoAggregate(ctx context.Context, obj store.Object, reason Reason, message string) error
	EventNoAggregateAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error
}

type NoopRecorder struct{}

func (NoopRecorder) Event(ctx context.Context, obj store.Object, reason Reason, message string) error {
	return nil
}

func (NoopRecorder) EventAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	return nil
}

func (NoopRecorder) EventNoAggregate(ctx context.Context, obj store.Object, reason Reason, message string) error {
	return nil
}

func (NoopRecorder) EventNoAggregateAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	return nil
}

type QueueRecorder struct {
	Cache chan Event
	Queue queue.Queue
}

func NewQueueRecorder(ctx context.Context, q queue.Queue, cachesize int64) Recorder {
	r := &QueueRecorder{
		Queue: q,
		Cache: make(chan Event, cachesize),
	}
	go r.run(ctx)
	return r
}

func (q *QueueRecorder) Event(ctx context.Context, obj store.Object, reason Reason, message string) error {
	return q.EventAnnotations(ctx, obj, reason, message, nil)
}

func (q *QueueRecorder) EventAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	return q.eventAggregate(ctx, obj, reason, message, annotations)
}

// EventNoAggregate records an event without aggregate
func (q *QueueRecorder) EventNoAggregate(ctx context.Context, obj store.Object, reason Reason, message string) error {
	return q.EventNoAggregateAnnotations(ctx, obj, reason, message, nil)
}

// EventNoAggregate records an event without aggregate
func (q *QueueRecorder) EventNoAggregateAnnotations(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	return q.eventNoAggregate(ctx, obj, reason, message, annotations)
}

func (q *QueueRecorder) eventAggregate(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	identifier := obj.GetUID() + "-" + string(reason) + "-" + message
	return q.event(ctx, identifier, obj, reason, message, annotations)
}

func (q *QueueRecorder) eventNoAggregate(ctx context.Context, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	identifier := obj.GetUID() + "-" + string(reason) + "-" + uuid.NewString()
	return q.event(ctx, identifier, obj, reason, message, annotations)
}

func (q *QueueRecorder) event(ctx context.Context, id string, obj store.Object, reason Reason, message string, annotations map[string]string) error {
	merged := mergeMap(obj.GetAnnotations(), annotations)

	e := Event{
		UID:               uuid.NewString(),
		Identifier:        id,
		Object:            store.ResourcedObjectReferenceFrom(obj),
		Annotations:       merged,
		Reason:            reason,
		Message:           message,
		CreationTimestamp: meta.Now(),
	}
	select {
	case q.Cache <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func mergeMap[M ~map[K]V, K comparable, V any](kvs ...M) M {
	ret := make(M)
	for _, kv := range kvs {
		maps.Copy(ret, kv)
	}
	return ret
}

func (q *QueueRecorder) run(ctx context.Context) error {
	log := log.FromContext(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-q.Cache:
			data, err := json.Marshal(e)
			if err != nil {
				return err
			}
			id := e.Identifier
			if id == "" {
				id = e.UID
			}
			if err := q.Queue.Enqueue(ctx, id, data, queue.EnqueueOptions{}); err != nil {
				if errors.IsAlreadyExists(err) {
					continue
				}
				log.Error(err, "enqueue event", "event", e)
			}
		}
	}
}
