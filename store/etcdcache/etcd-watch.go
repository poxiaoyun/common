package etcdcache

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/features"
	"k8s.io/apiserver/pkg/storage"
	storeerr "k8s.io/apiserver/pkg/storage/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

func init() {
	utilfeature.DefaultMutableFeatureGate.OverrideDefault(features.WatchList, true)
}

// Watch implements store.Store.
func (c *generic) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	options := &store.WatchOptions{}
	for _, opt := range opts {
		opt(options)
	}
	preficate, err := ConvertPredicate(options.LabelRequirements, options.FieldRequirements)
	if err != nil {
		return nil, err
	}
	if err := c.core.validateObject(obj); err != nil {
		return nil, err
	}
	resource, err := store.GetResource(obj)
	if err != nil {
		return nil, err
	}
	_, newItemFunc, err := store.NewItemFuncFromList(obj)
	if err != nil {
		return nil, err
	}
	prefix := getlistkey(c.scopes, resource)
	storageOptions := storage.ListOptions{Predicate: preficate, Recursive: true}
	// allow watch bookmarks to enabled watchlist
	if options.SendInitialEvents {
		storageOptions.SendInitialEvents = ptr.To(true)
		storageOptions.Predicate.AllowWatchBookmarks = true
	}
	if options.ResourceVersion != 0 {
		storageOptions.ResourceVersion = strconv.FormatInt(options.ResourceVersion, 10)
	}
	db := c.core.getResource(resource)
	watcher, err := db.storage.Watch(ctx, prefix, storageOptions)
	if err != nil {
		err = storeerr.InterpretWatchError(err, db.resource, "")
		return nil, err
	}
	cancelctx, cancel := context.WithCancel(ctx)
	ww := &warpWatcher{
		w:                watcher,
		cancel:           cancel,
		scopes:           c.scopes,
		resource:         db.resource,
		includeSubscopes: options.IncludeSubScopes,
		newItemFunc:      newItemFunc,
		result:           make(chan store.WatchEvent, 1),
	}
	go ww.run(cancelctx)

	return ww, nil
}

type warpWatcher struct {
	w                watch.Interface
	cancel           context.CancelFunc
	scopes           []store.Scope
	resource         schema.GroupResource
	includeSubscopes bool
	newItemFunc      func() store.Object
	result           chan store.WatchEvent
}

func (w *warpWatcher) run(ctx context.Context) {
	defer log.V(5).Info("watcher stopped", "resource", w.resource, "scopes", w.scopes)
	go func() {
		<-ctx.Done()
		w.w.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-w.w.ResultChan():
			if !ok {
				return
			}
			if e.Type == watch.Error {
				w.result <- store.WatchEvent{
					Error: fmt.Errorf("watch error: %v", e.Object),
				}
				return
			}
			if e.Type == watch.Bookmark {
				w.result <- store.WatchEvent{
					Type: store.WatchEventBookmark,
				}
				continue
			}
			uns, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				cachable, ok := e.Object.(runtime.CacheableObject)
				if !ok {
					w.result <- store.WatchEvent{
						Error: fmt.Errorf("object is not an unstructured.Unstructured, current type: %T", e.Object),
					}
					return
				}
				obj := cachable.GetObject()
				uns, ok = obj.(*unstructured.Unstructured)
				if !ok {
					w.result <- store.WatchEvent{
						Error: fmt.Errorf("cacheable object is not an unstructured.Unstructured, current type: %T", obj),
					}
					return
				}
			}

			newobj := w.newItemFunc()
			ConvertFromUnstructured(uns, newobj, w.resource)

			if !w.includeSubscopes && !store.ScopesEquals(newobj.GetScopes(), w.scopes) {
				continue
			}

			w.result <- store.WatchEvent{
				Type: func(et watch.EventType) store.WatchEventType {
					switch et {
					case watch.Added:
						return store.WatchEventCreate
					case watch.Modified:
						return store.WatchEventUpdate
					case watch.Deleted:
						return store.WatchEventDelete
					default:
						return store.WatchEventType(et)
					}
				}(e.Type),
				Object: newobj,
			}
		}
	}
}

func (w *warpWatcher) Events() <-chan store.WatchEvent {
	return w.result
}

func (w *warpWatcher) Stop() {
	w.cancel()
}
