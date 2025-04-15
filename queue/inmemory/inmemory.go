package inmemory

import (
	"context"
	"fmt"
	"time"

	"xiaoshiai.cn/common/queue"
)

func New() *InMemoryQueue {
	return &InMemoryQueue{
		ch: make(chan queueitem, 16),
	}
}

var _ queue.Queue = (*InMemoryQueue)(nil)

type InMemoryQueue struct {
	ch chan queueitem
}

type queueitem struct {
	id   string
	data []byte
}

// Consume implements queue.Queue.
func (i *InMemoryQueue) Consume(ctx context.Context, handler queue.QueueHandleFunc, options queue.ConsumeOptions) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case item := <-i.ch:
			if err := handler(ctx, item.id, item.data); err != nil {
				select {
				case i.ch <- item:
				default:
				}
				return err
			}
		}
	}
}

// Enqueue implements queue.Queue.
func (i *InMemoryQueue) Enqueue(ctx context.Context, id string, data []byte, options queue.EnqueueOptions) error {
	if id == "" {
		id = generateUniqueID()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case i.ch <- queueitem{id: id, data: data}:
		return nil
	}
}

// generateUniqueID creates a unique ID for queue items
func generateUniqueID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
