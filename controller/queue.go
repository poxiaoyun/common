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

func NewDefaultTypedQueue[T comparable](name string, rateLimiter workqueue.RateLimiter) TypedQueue[T] {
	if rateLimiter == nil {
		rateLimiter = workqueue.DefaultControllerRateLimiter()
	}
	return &DefaultTypedQueue[T]{
		Queue: workqueue.NewNamedRateLimitingQueue(rateLimiter, name),
	}
}

type DefaultTypedQueue[T any] struct {
	Queue workqueue.RateLimitingInterface
}

// AddAfter implements ControllerQueue.
func (w *DefaultTypedQueue[T]) AddAfter(key T, after time.Duration) {
	w.Queue.AddAfter(key, after)
}

// AddRateLimited implements ControllerQueue.
func (w *DefaultTypedQueue[T]) AddRateLimited(key T) {
	w.Queue.AddRateLimited(key)
}

// ShutDown implements ControllerQueue.
func (w *DefaultTypedQueue[T]) ShutDown() {
	w.Queue.ShutDown()
}

// Done implements ControllerQueue.
func (w *DefaultTypedQueue[T]) Done(item T) {
	w.Queue.Done(item)
}

// Forget implements ControllerQueue.
func (w *DefaultTypedQueue[T]) Forget(item T) {
	w.Queue.Forget(item)
}

// Get implements ControllerQueue.
func (w *DefaultTypedQueue[T]) Get() (T, bool) {
	item, shutdown := w.Queue.Get()
	if shutdown {
		return *new(T), shutdown
	}
	key, ok := item.(T)
	if !ok {
		w.Queue.Forget(item)
		return *new(T), false
	}
	return key, shutdown
}

// Add implements ControllerQueue.
func (w *DefaultTypedQueue[T]) Add(key T) {
	w.Queue.Add(key)
}
