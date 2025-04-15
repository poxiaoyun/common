package queue

import (
	"context"
	"time"
)

type EnqueueOptions struct {
	After time.Duration
}

type QueueHandleFunc func(ctx context.Context, id string, data []byte) error

type ConsumeOptions struct {
	MaxWorkers int
}

type Queue interface {
	// Enqueue enqueues a data to the queue.
	// if id is empty, it will be generated automatically.
	// when id set, multiple data with the same id will be replaced.
	// enqueue a consumed id will be ignored.
	Enqueue(ctx context.Context, id string, data []byte, options EnqueueOptions) error
	Consume(ctx context.Context, handler QueueHandleFunc, options ConsumeOptions) error
}
