package cache

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
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

// Count implements Store.
func (c *CacheStore) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	options := &store.CountOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if obj == nil {
		return 0, errors.NewBadRequest("object list is nil")
	}
	if obj.GetResource() == "" {
		return 0, errors.NewBadRequest("resource is required for object list")
	}
	resource := obj.GetResource()
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
	resource := obj.GetResource()
	if resource == "" {
		return errors.NewBadRequest("resource is required for object")
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
	resource := list.GetResource()
	if resource == "" {
		return errors.NewBadRequest("resource is required for object list")
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
	c := &cachedResource{resource: resource}
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
		key += "/" + scope.Resource + "/" + scope.Name
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
	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Cap:      30 * time.Second,
		Steps:    math.MaxInt32,
		Factor:   2.0,
		Jitter:   1.0,
	}
	wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		if err := c.sync(ctx, store); err != nil {
			log.Error(err, "sync failed", "resource", c.resource)
			// should retry on error
			return false, nil
		}
		return true, nil
	})
}

func (c *cachedResource) sync(ctx context.Context, s store.Store) error {
	newkvs := newThreadSafeReversionMap()
	if c.kvs == nil {
		c.kvs = newkvs
	}
	opts := []store.WatchOption{
		func(wo *store.WatchOptions) {
			wo.ResourceVersion = c.kvs.latestSyncRevision()
			wo.IncludeSubScopes = true
			wo.SendInitialEvents = true
		},
	}
	w, err := s.Watch(ctx, &store.List[store.Unstructured]{Resource: c.resource}, opts...)
	if err != nil {
		return err
	}
	defer w.Stop()

	initlized := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-w.Events():
			if event.Error != nil {
				return event.Error
			}
			if event.Type == store.WatchEventBookmark {
				if !initlized {
					initlized = true
					c.lock.Lock()
					c.initlized = true
					c.lock.Unlock()
				}
				continue
			}
			objval, ok := event.Object.(*store.Unstructured)
			if !ok {
				continue
			}
			objid := c.getkey(objval.GetScopes(), objval.GetName())
			switch event.Type {
			case store.WatchEventCreate:
				newkvs.set(objid, true, objval)
			case store.WatchEventUpdate:
				newkvs.set(objid, false, objval)
			case store.WatchEventDelete:
				newkvs.delete(objid, objval)
			}
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

func (c *threadsafeReversionMap) set(key string, iscreate bool, obj *store.Unstructured) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if latestversion := obj.GetResourceVersion(); c.lastSyncRevision < latestversion {
		c.lastSyncRevision = latestversion
	}
	c.items[key] = obj
	if iscreate {
		c.notify(key, store.WatchEventCreate, obj)
	} else {
		c.notify(key, store.WatchEventUpdate, obj)
	}
}

func (c *threadsafeReversionMap) delete(key string, obj *store.Unstructured) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if latestversion := obj.GetResourceVersion(); c.lastSyncRevision < latestversion {
		c.lastSyncRevision = latestversion
	}
	delete(c.items, key)
	c.notify(key, store.WatchEventDelete, obj)
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
	if strings.HasPrefix(key, t.prefix+"/") {
		t.on(key, kind, obj)
	}
}
