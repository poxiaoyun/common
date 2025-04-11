package cache

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/retry"
	"xiaoshiai.cn/common/store"
)

var _ store.Store = &CacheStore{}

func NewCacheStore(from store.Store) *CacheStore {
	return &CacheStore{
		core: &cacheStoreCore{
			ctx:       context.Background(),
			store:     from,
			resources: map[string]*cachedResource{},
		},
	}
}

type CacheStore struct {
	scopes []store.Scope
	core   *cacheStoreCore
}

// DeleteBatch implements store.Store.
func (g *CacheStore) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	return g.core.store.Scope(g.scopes...).DeleteBatch(ctx, obj, opts...)
}

// Count implements Store.
func (c *CacheStore) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return 0, err
	}
	options := &store.CountOptions{}
	for _, opt := range opts {
		opt(options)
	}
	// filter
	items, _, err := c.core.
		resource(resource).
		list(ctx, c.scopes, options.LabelRequirements, options.FieldRequirements)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// Create implements Store.
func (c *CacheStore) Create(ctx context.Context, obj store.Object, opts ...store.CreateOption) error {
	return c.core.store.Scope(c.scopes...).Create(ctx, obj, opts...)
}

// Delete implements Store.
func (g *CacheStore) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	return g.core.store.Scope(g.scopes...).Delete(ctx, obj, opts...)
}

// Get implements Store.
func (g *CacheStore) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}
	options := &store.GetOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if obj == nil {
		return errors.NewBadRequest("object is nil")
	}
	if _, err := store.EnforcePtr(obj); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object must be a pointer: %v", err))
	}
	if name == "" {
		return errors.NewBadRequest(fmt.Sprintf("name is required for %s", obj.GetResource()))
	}
	uns, err := g.core.resource(resource).get(ctx, g.scopes, name)
	if err != nil {
		return err
	}
	if options.ResourceVersion > 0 && uns.GetResourceVersion() < options.ResourceVersion {
		return errors.NewConflict(resource, name, fmt.Errorf("revision %d is too new", options.ResourceVersion))
	}
	if err := store.FromUnstructured(uns, obj); err != nil {
		return errors.NewInternalError(fmt.Errorf("failed to convert object: %w", err))
	}
	return nil
}

// List implements Store.
func (g *CacheStore) List(ctx context.Context, list store.ObjectList, opts ...store.ListOption) error {
	resource, err := store.GetResource(list)
	if err != nil {
		return err
	}
	options := &store.ListOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if list == nil {
		return errors.NewBadRequest("object list is nil")
	}
	if _, err := store.EnforcePtr(list); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object list must be a pointer: %v", err))
	}
	if options.ResourceVersion > 0 {
		return errors.NewBadRequest("list with resource version is not supported in cache store")
	}
	v, newItemFunc, err := store.NewItemFuncFromList(list)
	if err != nil {
		return err
	}
	// filter
	items, rev, err := g.core.
		resource(resource).
		list(ctx, g.scopes, options.LabelRequirements, options.FieldRequirements)
	if err != nil {
		return err
	}
	// sort
	sorts := store.ParseSorts(options.Sort)
	slices.SortFunc(items, func(a, b *store.Unstructured) int {
		return store.CompareUnstructuredField(a, b, sorts)
	})
	// page
	total := len(items)
	if options.Page > 0 && options.Size > 0 {
		skip := (options.Page - 1) * options.Size
		if skip < len(items) {
			items = items[skip:]
		} else {
			items = []*store.Unstructured{}
		}
	}
	if limit := options.Size; limit > 0 {
		if limit < len(items) {
			items = items[:limit]
		}
	}

	// decode
	// clean existing items
	v.SetZero()
	store.GrowSlice(v, len(items))
	for _, item := range items {
		obj := newItemFunc()
		if err := store.FromUnstructured(item, obj); err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to convert object: %w", err))
		}
		v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
	}
	list.SetResourceVersion(rev)
	list.SetToal(total)
	list.SetPage(options.Page)
	list.SetSize(options.Size)
	return nil
}

// Patch implements Store.
func (g *CacheStore) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	return g.core.store.Scope(g.scopes...).Patch(ctx, obj, patch, opts...)
}

// Update implements Store.
func (g *CacheStore) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	return g.core.store.Scope(g.scopes...).Update(ctx, obj, opts...)
}

// Scope implements Store.
func (g *CacheStore) Scope(scope ...store.Scope) store.Store {
	return &CacheStore{scopes: append(g.scopes, scope...), core: g.core}
}

// Status implements Store.
func (g *CacheStore) Status() store.StatusStorage {
	return &CacheStatusStore{scopes: g.scopes, core: g.core}
}

var _ store.StatusStorage = &CacheStatusStore{}

type CacheStatusStore struct {
	scopes []store.Scope
	core   *cacheStoreCore
}

// Patch implements StatusStorage.
func (c *CacheStatusStore) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	return c.core.store.Scope(c.scopes...).Patch(ctx, obj, patch, opts...)
}

// Update implements StatusStorage.
func (c *CacheStatusStore) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	return c.core.store.Scope(c.scopes...).Update(ctx, obj, opts...)
}

type cacheStoreCore struct {
	ctx       context.Context
	store     store.Store
	lock      sync.RWMutex
	resources map[string]*cachedResource
}

func (g *cacheStoreCore) resource(resource string) *cachedResource {
	g.lock.Lock()
	defer g.lock.Unlock()
	if c, ok := g.resources[resource]; ok {
		return c
	}
	c := &cachedResource{
		resource: resource,
		watchers: map[int64]*cachedWatcher{},
		kvs:      newThreadSafeReversionMap(),
	}
	go c.run(g.ctx, g.store)
	g.resources[resource] = c
	return c
}

type cachedResource struct {
	resource  string
	lock      sync.RWMutex
	initlized bool
	kvs       *threadsafeReversionMap

	watcherIndex atomic.Int64
	watcherLock  sync.RWMutex
	watchers     map[int64]*cachedWatcher
}

func (c *cachedResource) get(ctx context.Context, scopes []store.Scope, name string) (*store.Unstructured, error) {
	if err := c.waitSync(ctx); err != nil {
		return nil, err
	}
	objval, ok := c.kvs.get(c.getkey(scopes, name))
	if !ok {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return objval, nil
}

func (c *cachedResource) list(ctx context.Context, scopes []store.Scope,
	labelselector, fieldselector store.Requirements,
) ([]*store.Unstructured, int64, error) {
	return c.listPrefix(ctx, c.getlistkey(scopes), labelselector, fieldselector)
}

func (c *cachedResource) listPrefix(ctx context.Context, prefix string,
	labelselector, fieldselector store.Requirements,
) ([]*store.Unstructured, int64, error) {
	if err := c.waitSync(ctx); err != nil {
		return nil, 0, err
	}
	objs, rev := c.kvs.list(prefix)
	items := []*store.Unstructured{}
	for _, obj := range objs {
		if !store.MatchLabelReqirements(obj, labelselector) {
			continue
		}
		if !store.MatchUnstructuredFieldRequirments(obj, fieldselector) {
			continue
		}
		items = append(items, obj)
	}
	return items, rev, nil
}

func (e *cachedResource) getlistkey(scopes []store.Scope) string {
	key := e.resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	return key + "/"
}

func (e *cachedResource) getkey(scopes []store.Scope, name string) string {
	key := e.resource
	for _, scope := range scopes {
		key += ("/" + scope.Resource + "/" + scope.Name)
	}
	return key + "/" + name
}

func (c *cachedResource) waitSync(ctx context.Context) error {
	if c.initlized {
		return nil
	}
	interval := 500 * time.Millisecond
	for {
		if c.initlized {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		log.Info("waiting for cache resource to sync", "resource", c.resource)
		time.Sleep(interval)
	}
}

func (c *cachedResource) run(ctx context.Context, store store.Store) {
	log.Info("start syncing cache resource", "resource", c.resource)
	retry.OnError(ctx, func(ctx context.Context) error {
		return c.sync(ctx, store)
	})
}

func (c *cachedResource) sync(ctx context.Context, s store.Store) error {
	log := log.FromContext(ctx)

	opts := []store.WatchOption{
		func(wo *store.WatchOptions) {
			wo.ResourceVersion = c.kvs.latestSyncRevision()
			wo.IncludeSubScopes = true
			wo.SendInitialEvents = true
		},
	}
	log.Info("start watching cache resource", "resource", c.resource)

	w, err := s.Watch(ctx, &store.List[store.Unstructured]{Resource: c.resource}, opts...)
	if err != nil {
		return err
	}
	defer w.Stop()

	firstbookmark := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.Events():
			if !ok {
				return fmt.Errorf("watcher channel closed")
			}
			if event.Error != nil {
				return event.Error
			}
			if event.Type == store.WatchEventBookmark {
				if !firstbookmark {
					firstbookmark = true
					log.Info("cache resource synced", "resource", c.resource)
					if !c.initlized {
						c.initlized = true
					}
				}
				continue
			}
			objval, ok := event.Object.(*store.Unstructured)
			if !ok {
				continue
			}
			objid := c.getkey(objval.GetScopes(), objval.GetName())
			c.kvs.set(objid, event.Type, objval)
		}
	}
}

func newThreadSafeReversionMap() *threadsafeReversionMap {
	return &threadsafeReversionMap{
		items:    map[string]*store.Unstructured{},
		watchers: map[int64]*threadsafeReversionMapWacther{},
	}
}

type threadsafeReversionMap struct {
	lock             sync.RWMutex
	items            map[string]*store.Unstructured
	lastSyncRevision int64

	watcherID atomic.Int64
	watchers  map[int64]*threadsafeReversionMapWacther
}

func (c *threadsafeReversionMap) latestSyncRevision() int64 {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.lastSyncRevision
}

func (c *threadsafeReversionMap) get(key string) (*store.Unstructured, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	obj, ok := c.items[key]
	return obj, ok
}

func (c *threadsafeReversionMap) list(prefix string) ([]*store.Unstructured, int64) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	objs := make([]*store.Unstructured, 0, len(c.items))
	for key, obj := range c.items {
		if strings.HasPrefix(key, prefix) {
			objs = append(objs, obj)
		}
	}
	return objs, c.lastSyncRevision
}

func (c *threadsafeReversionMap) notify(key string, kind store.WatchEventType, obj *store.Unstructured) {
	for _, w := range c.watchers {
		w.send(key, kind, obj)
	}
}

func (c *threadsafeReversionMap) set(key string, event store.WatchEventType, obj *store.Unstructured) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if latestversion := obj.GetResourceVersion(); c.lastSyncRevision < latestversion {
		c.lastSyncRevision = latestversion
	}
	switch event {
	case store.WatchEventCreate:
		exists, ok := c.items[key]
		if ok && exists.GetResourceVersion() >= obj.GetResourceVersion() {
			return
		}
		c.items[key] = obj
	case store.WatchEventUpdate:
		exists, ok := c.items[key]
		if !ok || exists.GetResourceVersion() >= obj.GetResourceVersion() {
			return
		}
		c.items[key] = obj
	case store.WatchEventDelete:
		if _, ok := c.items[key]; !ok {
			return
		}
		delete(c.items, key)
	}
	c.notify(key, event, obj)
}

func (c *threadsafeReversionMap) watch(ctx context.Context, prefix string, on func(key string, kind store.WatchEventType, obj *store.Unstructured)) {
	// lock here to block new incoming events
	c.lock.Lock()
	w := &threadsafeReversionMapWacther{
		id:     c.watcherID.Add(1),
		prefix: prefix,
		on:     on,
	}
	for key, obj := range c.items {
		w.send(key, store.WatchEventCreate, obj)
	}
	c.watchers[w.id] = w
	c.lock.Unlock()

	<-ctx.Done()
	c.lock.Lock()
	delete(c.watchers, w.id)
	c.lock.Unlock()
}

type threadsafeReversionMapWacther struct {
	id     int64
	prefix string
	on     func(key string, kind store.WatchEventType, obj *store.Unstructured)
}

func (t *threadsafeReversionMapWacther) send(key string, kind store.WatchEventType, obj *store.Unstructured) {
	if strings.HasPrefix(key, t.prefix) {
		t.on(key, kind, obj)
	}
}
