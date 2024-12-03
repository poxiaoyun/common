package controller

import (
	"time"

	"k8s.io/client-go/util/workqueue"
)

type TypedQueue[T comparable] interface {
	Add(key T)
	Get() (item T, shutdown bool)
	Done(item T)
	Forget(item T)
	ShutDown()
	AddAfter(key T, after time.Duration)
	AddRateLimited(key T)
}

func NewDefaultTypedQueue[T comparable](name string, rateLimiter workqueue.TypedRateLimiter[T]) TypedQueue[T] {
	if rateLimiter == nil {
		rateLimiter = workqueue.DefaultTypedControllerRateLimiter[T]()
	}
	return workqueue.NewTypedRateLimitingQueueWithConfig(rateLimiter, workqueue.TypedRateLimitingQueueConfig[T]{Name: name})
}

type DefaultTypedQueue[T comparable] workqueue.TypedRateLimitingInterface[T]
