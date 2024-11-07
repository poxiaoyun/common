package cache

import (
	"context"
	"fmt"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/etcd"
)

// Watch implements Store.
func (g *CacheStore) Watch(ctx context.Context, list store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	options := &store.WatchOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if list == nil {
		return nil, errors.NewBadRequest("object list is nil")
	}
	if _, err := store.EnforcePtr(list); err != nil {
		return nil, errors.NewBadRequest(fmt.Sprintf("object list must be a pointer: %v", err))
	}
	resource := list.GetResource()
	if resource == "" {
		return nil, errors.NewBadRequest("resource is required for object list")
	}
	if options.ResourceVersion > 0 {
		return nil, errors.NewBadRequest("watch with resource version is not supported in cache store")
	}
	_, newItemFunc, err := store.NewItemFuncFromList(list)
	if err != nil {
		return nil, err
	}
	return g.core.resource(resource).watch(ctx, g.scopes, newItemFunc, options.LabelRequirements, options.FieldRequirements)
}

func (c *cachedResource) watch(ctx context.Context,
	scopes []store.Scope, newfunc func() store.Object,
	labelselector, fieldselector store.Requirements,
) (store.Watcher, error) {
	if err := c.waitSync(ctx); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &cachedWatcher{
		id:            c.watcherIndex.Add(1),
		parent:        c,
		fieldSelector: fieldselector,
		labelSelector: labelselector,
		scopes:        scopes,
		newitem:       newfunc,
		resultChan:    make(chan store.WatchEvent, 100),
		cancel:        cancel,
	}
	go w.run(ctx)

	c.watcherLock.Lock()
	defer c.watcherLock.Unlock()

	c.watchers[w.id] = w
	return w, nil
}

func (c *cachedResource) detachWatcher(w *cachedWatcher) {
	c.watcherLock.Lock()
	defer c.watcherLock.Unlock()
	delete(c.watchers, w.id)
}

var _ store.Watcher = &cachedWatcher{}

type cachedWatcher struct {
	id     int64
	parent *cachedResource
	cancel context.CancelFunc

	fieldSelector store.Requirements
	labelSelector store.Requirements
	scopes        []store.Scope

	latestRev  int64
	newitem    func() store.Object
	resultChan chan store.WatchEvent
}

func (c *cachedWatcher) run(ctx context.Context) {
	defer c.parent.detachWatcher(c)
	prefix := c.parent.getlistkey(c.scopes)
	c.parent.kvs.watch(ctx, prefix, func(_ string, kind store.WatchEventType, obj *store.Unstructured) {
		c.send(ctx, kind, obj)
	})
}

func (c *cachedWatcher) send(ctx context.Context, kind store.WatchEventType, obj *store.Unstructured) {
	// filter
	if !store.ScopesEquals(obj.GetScopes(), c.scopes) {
		return
	}
	if !etcd.MatchLabelReqirements(obj, c.labelSelector) {
		return
	}
	if !etcd.MatchUnstructuredFieldRequirments(obj, c.fieldSelector) {
		return
	}
	// decode
	newobj := c.newitem()
	if err := store.FromUnstructured(obj, newobj); err != nil {
		log.Error(err, "failed to convert object", "object", obj)
		return
	}
	select {
	case c.resultChan <- store.WatchEvent{Type: kind, Object: newobj}:
	case <-ctx.Done():
	}
}

// Stop implements Watcher.
func (c *cachedWatcher) Stop() {
	c.cancel()
}

// Events implements Watcher.
func (c *cachedWatcher) Events() <-chan store.WatchEvent {
	return c.resultChan
}
