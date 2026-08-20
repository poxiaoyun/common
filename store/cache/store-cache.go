package cache

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

var _ store.PingableStore = &CacheStore{}

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

// Schema implements store.Store.
func (g *CacheStore) Schema() *store.Schema {
	return g.core.store.Schema()
}

// Capabilities implements store.Store. The cache owns page pagination and
// otherwise reports only the optional Watch behavior it delegates to the
// underlying store.
func (g *CacheStore) Capabilities() store.Capabilities {
	return store.Capabilities{Page: true, Watch: g.core.store.Capabilities().Watch}
}

func (g *CacheStore) Ping(ctx context.Context) error {
	pinger, ok := g.core.store.(store.Pinger)
	if !ok {
		return errors.NewNotImplemented("underlying store does not support ping")
	}
	return pinger.Ping(ctx)
}

// PatchBatch implements store.Store.
func (g *CacheStore) PatchBatch(ctx context.Context, obj store.ObjectList, patch store.PatchBatch, opts ...store.PatchBatchOption) error {
	return g.core.store.Scope(g.scopes...).PatchBatch(ctx, obj, patch, opts...)
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
	options := store.ApplyCountOptions(opts)
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
	options := store.ApplyGetOptions(opts)
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
	rev := ptr.Deref(options.ResourceVersion, int64(0))
	if rev > 0 && uns.GetResourceVersion() < rev {
		return errors.NewConflict(resource, name, fmt.Errorf("revision %d is too new", rev))
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
	options := store.ApplyListOptions(opts)
	if options.Limit > 0 {
		return errors.NewUnsupported("cache store does not support continuation pagination")
	}
	if list == nil {
		return errors.NewBadRequest("object list is nil")
	}
	if _, err := store.EnforcePtr(list); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object list must be a pointer: %v", err))
	}
	if options.ResourceVersion != nil {
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
	page := max(options.Page, 1)
	if options.Size > 0 {
		pageIndex := page - 1
		start := total
		if pageIndex <= total/options.Size {
			start = pageIndex * options.Size
		}
		end := start + min(options.Size, total-start)
		items = items[start:end]
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
	if options.Size > 0 {
		store.SetPageListMetadata(list, page, options.Size, total)
	} else {
		store.SetUnpaginatedListMetadata(list, total)
	}
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
		ready:    make(chan struct{}),
		items:    map[string]*store.Unstructured{},
		watchers: map[int64]*cachedWatcher{},
	}
	go c.run(g.ctx, g.store)
	g.resources[resource] = c
	return c
}

type cachedResource struct {
	resource string

	stateLock     sync.RWMutex
	ready         chan struct{}
	isReady       bool
	terminalError error
	items         map[string]*store.Unstructured
	nextWatcherID int64
	watchers      map[int64]*cachedWatcher
}

func (c *cachedResource) get(ctx context.Context, scopes []store.Scope, name string) (*store.Unstructured, error) {
	if err := c.waitUntilReady(ctx); err != nil {
		return nil, err
	}
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()
	objval, ok := c.items[c.getObjectKey(scopes, name)]
	if !ok {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return objval, nil
}

func (c *cachedResource) list(ctx context.Context, scopes []store.Scope,
	labelselector, fieldselector store.Requirements,
) ([]*store.Unstructured, int64, error) {
	if err := c.waitUntilReady(ctx); err != nil {
		return nil, 0, err
	}
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()
	items := []*store.Unstructured{}
	for _, obj := range c.items {
		if !store.ScopesEquals(obj.GetScopes(), scopes) {
			continue
		}
		if !store.MatchLabelReqirements(obj, labelselector) {
			continue
		}
		if !store.MatchUnstructuredFieldRequirments(obj, fieldselector) {
			continue
		}
		items = append(items, obj)
	}
	return items, 0, nil
}

func (c *cachedResource) getObjectKey(scopes []store.Scope, name string) string {
	key := c.resource
	for _, scope := range scopes {
		key += ("/" + scope.Resource + "/" + scope.Name)
	}
	return key + "/" + name
}

func (c *cachedResource) waitUntilReady(ctx context.Context) error {
	for {
		c.stateLock.RLock()
		if c.isReady {
			c.stateLock.RUnlock()
			return nil
		}
		if c.terminalError != nil {
			err := c.terminalError
			c.stateLock.RUnlock()
			return err
		}
		ready := c.ready
		c.stateLock.RUnlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
	}
}

func (c *cachedResource) run(ctx context.Context, storage store.Store) {
	log.Info("start syncing cache resource", "resource", c.resource)
	list := &store.List[store.Unstructured]{Resource: c.resource}
	reflector := NewReflector(storage, list, store.WithSubScopes())
	handler := ReflectorHandlerFuncs[store.Unstructured]{
		ReplaceFunc:    c.replace,
		ApplyFunc:      c.apply,
		InvalidateFunc: c.invalidate,
	}
	if err := reflector.Run(ctx, handler); err != nil {
		c.fail(err)
	}
}

func (c *cachedResource) replace(_ context.Context, objects []*store.Unstructured) error {
	items := make(map[string]*store.Unstructured, len(objects))
	for _, object := range objects {
		items[c.getObjectKey(object.GetScopes(), object.GetID())] = object
	}
	c.stateLock.Lock()
	c.items = items
	c.terminalError = nil
	if !c.isReady {
		c.isReady = true
		close(c.ready)
	}
	c.stateLock.Unlock()
	log.Info("cache resource synced", "resource", c.resource)
	return nil
}

func (c *cachedResource) apply(_ context.Context, eventType store.WatchEventType, object *store.Unstructured) error {
	key := c.getObjectKey(object.GetScopes(), object.GetID())
	c.stateLock.Lock()
	old, exists := c.items[key]
	var current *store.Unstructured
	switch eventType {
	case store.WatchEventCreate, store.WatchEventUpdate:
		if exists && old.GetUID() == object.GetUID() && old.GetResourceVersion() >= object.GetResourceVersion() {
			c.stateLock.Unlock()
			return nil
		}
		c.items[key] = object
		current = object
	case store.WatchEventDelete:
		if !exists || old.GetUID() != object.GetUID() || old.GetResourceVersion() > object.GetResourceVersion() {
			c.stateLock.Unlock()
			return nil
		}
		delete(c.items, key)
	default:
		c.stateLock.Unlock()
		return fmt.Errorf("cache event type %q is not an object mutation", eventType)
	}
	for id, watcher := range c.watchers {
		if watcher.enqueueTransition(old, current) {
			continue
		}
		delete(c.watchers, id)
		watcher.expire(errors.NewResourceExpired(c.resource, "watcher fell behind"))
	}
	c.stateLock.Unlock()
	return nil
}

func (c *cachedResource) invalidate(_ context.Context, cause error) {
	c.stateLock.Lock()
	watchers := c.watchers
	c.watchers = map[int64]*cachedWatcher{}
	if c.isReady {
		c.isReady = false
		c.ready = make(chan struct{})
	}
	c.stateLock.Unlock()
	for _, watcher := range watchers {
		watcher.expire(errors.NewResourceExpired(c.resource, cause.Error()))
	}
}

func (c *cachedResource) fail(err error) {
	c.stateLock.Lock()
	c.terminalError = err
	if !c.isReady {
		close(c.ready)
	}
	c.stateLock.Unlock()
}
