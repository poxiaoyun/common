package queue

import (
	"context"
	"time"
)

type EnqueueOptions struct {
	After time.Duration
}

type QueueHandleFunc func(ctx context.Context, id string, data string) error

type ConsumeOptions struct {
	MaxWorkers int
}

type Queue interface {
	Enqueue(ctx context.Context, data string, options EnqueueOptions) error
	Consume(ctx context.Context, handler QueueHandleFunc, options ConsumeOptions) error
}
