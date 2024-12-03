package controller

import (
	"context"
	"errors"
	"fmt"

	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type Source[K comparable] interface {
	Run(ctx context.Context, queue TypedQueue[K]) error
}

type SourceFunc[K comparable] func(ctx context.Context, queue TypedQueue[K]) error

func (f SourceFunc[K]) Run(ctx context.Context, queue TypedQueue[K]) error {
	return f(ctx, queue)
}

func NewStoreSource(storage store.Store, example store.Object) *StoreSource {
	return NewCustomStoreSource(storage, example, func(ctx context.Context, kind store.WatchEventType, obj store.Object) ([]ScopedKey, error) {
		return []ScopedKey{ScopedKeyFromObject(obj)}, nil
	})
}

func NewCustomStoreSource(storage store.Store, example store.Object, keyfunc KeyFunc) *StoreSource {
	resource, err := store.GetResource(example)
	if err != nil {
		panic(err)
	}
	return &StoreSource{
		Store:    storage,
		Resource: resource,
		KeyFunc:  keyfunc,
	}
}

type KeyFunc func(ctx context.Context, kind store.WatchEventType, obj store.Object) ([]ScopedKey, error)

type StoreSource struct {
	store.Store
	Resource string
	KeyFunc  KeyFunc
}

func (s *StoreSource) Run(ctx context.Context, queue TypedQueue[ScopedKey]) error {
	logger := log.FromContext(ctx).WithValues("resource", s.Resource)
	logger.Info("source start")
	ctx = log.NewContext(ctx, logger)
	return RunListWatchContext(ctx, s.Store, s.Resource, EventHandlerFunc[*store.Unstructured](func(ctx context.Context, kind store.WatchEventType, obj *store.Unstructured) error {
		logger.Info("event", "kind", kind, "name", obj.GetName())

		keys, err := s.KeyFunc(ctx, kind, obj)
		if err != nil {
			logger.Error(err, "key error")
			return nil
		}
		for i := range keys {
			queue.Add(keys[i])
		}
		return nil
	}))
}

type EventHandler[T store.Object] interface {
	OnEvent(ctx context.Context, kind store.WatchEventType, obj T) error
}

var _ EventHandler[store.Object] = EventHandlerFunc[store.Object](nil)

type EventHandlerFunc[T store.Object] func(ctx context.Context, kind store.WatchEventType, obj T) error

func (f EventHandlerFunc[T]) OnEvent(ctx context.Context, kind store.WatchEventType, obj T) error {
	return f(ctx, kind, obj)
}

func RunListWatchContext(ctx context.Context, storage store.Store, resource string, handler EventHandler[*store.Unstructured]) error {
	return RetryOnError(ctx, DefaultRetry, AlwaysRetry, func(ctx context.Context) error {
		return RunListWatch(ctx, storage, resource, true, handler)
	})
}

func RunListWatch(ctx context.Context, storage store.Store, resource string, subScope bool, handler EventHandler[*store.Unstructured]) error {
	list := &store.List[store.Unstructured]{}
	list.SetResource(resource)

	// list
	if false {
		// our watch returns list and later changes in a single watch
		// so we do not need list once anymore
		if err := storage.List(ctx, list, store.WithSubScopes()); err != nil {
			return err
		}
		for _, obj := range list.Items {
			if err := handler.OnEvent(ctx, store.WatchEventCreate, &obj); err != nil {
				return fmt.Errorf("handler error: %w", err)
			}
		}
	}
	// watch
	inlcudesubscope := func(wo *store.WatchOptions) {
		wo.IncludeSubScopes = subScope
		wo.ResourceVersion = list.ResourceVersion
	}
	watcher, err := storage.Watch(ctx, list, inlcudesubscope)
	if err != nil {
		return err
	}
	defer func() {
		watcher.Stop()
		log.Info("watcher stoped")
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events():
			if !ok {
				return fmt.Errorf("watcher channel closed")
			}
			switch event.Type {
			case store.WatchEventCreate, store.WatchEventUpdate, store.WatchEventDelete:
				obj, ok := event.Object.(*store.Unstructured)
				if !ok {
					log.Error(errors.New("watch event value is not T"), "watch error")
					return errors.New("watch event value is not T")
				}
				if event.Error != nil {
					log.Error(event.Error, "watch error")
					return event.Error
				}
				log.V(5).Info("watch event", "type", event.Type, "name", obj.GetName(), "resource", obj.GetResource())
				if err := handler.OnEvent(ctx, event.Type, obj); err != nil {
					log.Error(err, "handle error")
					return err
				}
			case store.WatchEventBookmark:
				// ignore
			default:
				log.Info("unknown event type", "type", event.Type)
			}
		}
	}
}
