package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type ScopedKey struct {
	Name   string
	Prefix string
}

func ScopedKeyFromObject(obj store.Object) ScopedKey {
	return NewScopedKey(obj.GetScopes(), obj.GetName())
}

func NewScopedKey(scopes []store.Scope, name string) ScopedKey {
	return ScopedKey{Name: name, Prefix: EncodeScopes(scopes)}
}

func (s ScopedKey) Scopes() []store.Scope {
	return DecodeScopes(s.Prefix)
}

type ResourceScopedKey struct {
	Resource string
	Name     string
	Prefix   string
}

func EncodeScopes(scopes []store.Scope) string {
	ret := ""
	for _, scope := range scopes {
		ret += "/" + scope.Resource + "/" + scope.Name
	}
	return ret
}

func DecodeScopes(scopes string) []store.Scope {
	if len(scopes) == 0 {
		return nil
	}
	ret := []store.Scope{}
	parts := strings.Split(scopes, "/")
	if parts[0] == "" {
		parts = parts[1:]
	}
	for i := 0; i < len(parts); i += 2 {
		ret = append(ret, store.Scope{
			Resource: parts[i],
			Name:     parts[i+1],
		})
	}
	return ret
}

type TypedReconciler[T any] interface {
	Reconcile(ctx context.Context, key T) (Result, error)
}

type Controller = TypedController[ScopedKey]

// InitalizeReconciler is an optional interface that can be implemented by a Reconciler to
type InitializeReconciler interface {
	Initialize(ctx context.Context) error
}

var _ TypedReconciler[any] = TypedReconcilerFunc[any](nil)

type TypedReconcilerFunc[T any] func(ctx context.Context, key T) (Result, error)

func (f TypedReconcilerFunc[T]) Reconcile(ctx context.Context, key T) (Result, error) {
	return f(ctx, key)
}

type Result struct {
	Requeue      bool
	RequeueAfter time.Duration
}

type ControllerOptions[T comparable] struct {
	Concurrent     int
	LeaderElection LeaderElection
	RateLimiter    workqueue.TypedRateLimiter[T]
}

type ControllerOption[T comparable] func(*ControllerOptions[T])

func WithConcurrent[T comparable](concurrent int) ControllerOption[T] {
	return func(o *ControllerOptions[T]) {
		o.Concurrent = concurrent
	}
}

func WithLeaderElection[T comparable](leader LeaderElection) ControllerOption[T] {
	return func(o *ControllerOptions[T]) {
		o.LeaderElection = leader
	}
}

func NewController(name string, sync TypedReconciler[ScopedKey], options ...ControllerOption[ScopedKey]) *TypedController[ScopedKey] {
	return NewTypedController(name, sync, options...)
}

func NewTypedController[T comparable](name string, sync TypedReconciler[T], options ...ControllerOption[T]) *TypedController[T] {
	opts := ControllerOptions[T]{}
	for _, opt := range options {
		opt(&opts)
	}
	opts.Concurrent = max(opts.Concurrent, 1)
	if sync == nil {
		panic("sync function is required")
	}
	c := &TypedController[T]{
		name:     name,
		options:  opts,
		queue:    NewDefaultTypedQueue(name, opts.RateLimiter),
		syncFunc: sync,
		ratelimiter: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[T](),
			workqueue.TypedRateLimitingQueueConfig[T]{Name: name + "-rate-limiter"},
		),
	}
	return c
}

type TypedController[T comparable] struct {
	name     string
	options  ControllerOptions[T]
	sources  []Source[T]
	queue    TypedQueue[T]
	syncFunc TypedReconciler[T]

	// ratelimiter is a global ratelimiter for the controller
	ratelimiter workqueue.TypedRateLimitingInterface[T]
}

func (h *TypedController[T]) Watch(souce ...Source[T]) *TypedController[T] {
	h.sources = append(h.sources, souce...)
	return h
}

func (h *TypedController[T]) Name() string {
	return h.name
}

func (h *TypedController[T]) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("controller", h.name)

	ctx = log.NewContext(ctx, logger)
	if h.options.LeaderElection != nil {
		return h.options.LeaderElection.OnLeader(ctx, 30*time.Second, func(ctx context.Context) error {
			logger.Info("starting controller on leader")
			return h.run(ctx)
		})
	} else {
		logger.Info("starting controller")
		return h.run(ctx)
	}
}

func (h *TypedController[T]) run(ctx context.Context) error {
	if init, ok := h.syncFunc.(InitializeReconciler); ok {
		if err := init.Initialize(ctx); err != nil {
			return err
		}
	}
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

func RunQueueConsumer[T comparable](ctx context.Context, queue TypedQueue[T], syncfunc func(ctx context.Context, key T) (Result, error), concurent int) error {
	go func() {
		<-ctx.Done()
		queue.ShutDown()
	}()

	log := log.FromContext(ctx)

	// 10 qps, burst 100
	ratelimit := rate.NewLimiter(rate.Limit(10), 100)

	// get item from queue and process
	wg := sync.WaitGroup{}
	wg.Add(concurent)
	for range concurent {
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
					// ratelimit 1
					ratelimit.Wait(ctx)

					result, err := syncfunc(ctx, val)
					if err != nil {
						log.Error(err, "sync", "key", val)
						queue.AddRateLimited(val)
					} else {
						if result.Requeue {
							queue.AddAfter(val, result.RequeueAfter)
						} else {
							queue.Forget(val)
						}
					}
					queue.Done(val)
				}
			}
		}()
	}
	wg.Wait()
	return nil
}
