package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/util/workqueue"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type ScopedKey struct {
	Name     string
	Resource string
	Scopes   []store.Scope
}

type (
	Controller           = TypedController[*ScopedKey]
	ControllerReconciler interface {
		Reconcile(ctx context.Context, key *ScopedKey) error
	}
	ControllerQueue = TypedQueue[*ScopedKey]
)

type TypedReconciler[T any] interface {
	Reconcile(ctx context.Context, key T) error
}

var _ TypedReconciler[any] = TypedReconcilerFunc[any](nil)

type TypedReconcilerFunc[T any] func(ctx context.Context, key T) error

func (f TypedReconcilerFunc[T]) Reconcile(ctx context.Context, key T) error {
	return f(ctx, key)
}

type ReQueueError struct {
	Err   error
	Atfer time.Duration
}

func (r ReQueueError) Error() string {
	if r.Err == nil {
		return fmt.Sprintf("retry after %s", r.Atfer)
	}
	return fmt.Sprintf("retry after %s: %s", r.Atfer, r.Err.Error())
}

func WithReQueue(after time.Duration, err error) error {
	if err == nil {
		return ReQueueError{Atfer: after}
	}
	return ReQueueError{Err: err, Atfer: after}
}

type ControllerOptions struct {
	Concurrent     int
	LeaderElection LeaderElection
	RateLimiter    workqueue.RateLimiter
}

type ControllerOption func(*ControllerOptions)

func WithConcurrent(concurrent int) ControllerOption {
	return func(o *ControllerOptions) {
		o.Concurrent = concurrent
	}
}

func WithLeaderElection(leader LeaderElection) ControllerOption {
	return func(o *ControllerOptions) {
		o.LeaderElection = leader
	}
}

func NewController(name string, sync ControllerReconciler, options ...ControllerOption) *Controller {
	return NewTypedController(name, sync, options...)
}

func NewTypedController[T comparable](name string, sync TypedReconciler[T], options ...ControllerOption) *TypedController[T] {
	opts := ControllerOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	if opts.Concurrent <= 0 {
		opts.Concurrent = 1
	}
	if sync == nil {
		panic("sync function is required")
	}
	c := &TypedController[T]{
		name:     name,
		options:  opts,
		queue:    NewDefaultTypedQueue[T](name, opts.RateLimiter),
		syncFunc: sync,
	}
	return c
}

type TypedController[T comparable] struct {
	name     string
	options  ControllerOptions
	sources  []Source[T]
	queue    TypedQueue[T]
	syncFunc TypedReconciler[T]
}

func (h *TypedController[T]) Watch(souce ...Source[T]) *TypedController[T] {
	h.sources = append(h.sources, souce...)
	return h
}

func (h *TypedController[T]) Name() string {
	return h.name
}

func (h *TypedController[T]) Run(ctx context.Context) error {
	ctx = log.NewContext(ctx, log.FromContext(ctx).WithName(h.name))
	log := log.FromContext(ctx)
	if h.options.LeaderElection != nil {
		return h.options.LeaderElection.OnLeader(ctx, h.name, 30*time.Second, func(ctx context.Context) error {
			log.Info("starting controller on leader")
			return h.run(ctx)
		})
	} else {
		log.Info("starting controller")
		return h.run(ctx)
	}
}

func (h *TypedController[T]) run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	// watch sources
	for _, source := range h.sources {
		source := source
		eg.Go(func() error {
			return source.Run(ctx, h.queue)
		})
	}
	// run queue consumer
	eg.Go(func() error {
		return RunQueueConsumer(ctx, h.queue, h.syncFunc.Reconcile, h.options.Concurrent)
	})
	return eg.Wait()
}

func RunQueueConsumer[T comparable](ctx context.Context, queue TypedQueue[T], syncfunc func(ctx context.Context, key T) error, concurent int) error {
	go func() {
		<-ctx.Done()
		queue.ShutDown()
		log.FromContext(ctx).Info("queue shutdown")
	}()

	logger := log.FromContext(ctx)

	// get item from queue and process
	wg := sync.WaitGroup{}
	wg.Add(concurent)
	for i := 0; i < concurent; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					val, stop := queue.Get()
					if stop {
						queue.Done(val)
						return
					}
					if err := syncfunc(ctx, val); err != nil {
						logger.Error(err, "sync error")
						// requeue
						retry := ReQueueError{}
						if errors.As(err, &retry) {
							queue.AddAfter(val, retry.Atfer)
						} else {
							queue.AddRateLimited(val)
						}
					} else {
						queue.Forget(val)
					}
					queue.Done(val)
				}
			}
		}()
	}
	wg.Wait()
	return nil
}
