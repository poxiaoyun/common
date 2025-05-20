package inmemory

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

var _ store.Store = &InMemory{}

type InMemory struct {
	core   *inmemory
	scopes []store.Scope
	status bool
}

// PatchBatch implements store.Store.
func (i *InMemory) PatchBatch(ctx context.Context, obj store.ObjectList, patch store.PatchBatch, opts ...store.PatchBatchOption) error {
	return errors.NewNotImplemented("batch patch is not supported")
}

func (i *InMemory) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	return 0, errors.NewNotImplemented("count is not supported")
}

func (i *InMemory) Create(ctx context.Context, obj store.Object, opts ...store.CreateOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.create(resources, i.scopes, obj.GetName(), obj)
	})
}

func (i *InMemory) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.delete(resources, i.scopes, obj.GetName(), nil)
	})
}

func (i *InMemory) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	return errors.NewNotImplemented("delete batch is not supported")
}

func (i *InMemory) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.get(resources, i.scopes, obj.GetName(), obj)
	})
}

func (i *InMemory) List(ctx context.Context, list store.ObjectList, opts ...store.ListOption) error {
	return errors.NewNotImplemented("list is not supported")
}

func (i *InMemory) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	return errors.NewNotImplemented("patch is not supported")
}

func (i *InMemory) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.put(resources, i.scopes, obj.GetName(), obj)
	})
}

func (i *InMemory) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	return nil, errors.NewNotImplemented("watch is not supported")
}

func (i *InMemory) Scope(scope ...store.Scope) store.Store {
	return &InMemory{core: i.core, scopes: append(i.scopes, scope...), status: i.status}
}

func (i *InMemory) Status() store.StatusStorage {
	return &InMemory{core: i.core, scopes: i.scopes, status: true}
}

type inmemory struct {
	mu  sync.RWMutex
	rev atomic.Uint64
	kvs map[string]kv
}

type kv struct {
	value []byte
	rev   uint64
}

func (m *inmemory) create(resource string, scopes []store.Scope, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.NewInternalError(err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := getkey(resource, scopes, name)
	if _, ok := m.kvs[key]; ok {
		return errors.NewAlreadyExists(resource, name)
	}
	m.kvs[key] = kv{
		value: data,
		rev:   m.rev.Add(1),
	}
	return nil
}

func (m *inmemory) get(resource string, scopes []store.Scope, name string, into store.Object) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := getkey(resource, scopes, name)
	kv, ok := m.kvs[key]
	if !ok {
		return errors.NewNotFound(resource, name)
	}
	if into != nil {
		if err := json.Unmarshal(kv.value, into); err != nil {
			return errors.NewInternalError(err)
		}
		into.SetResourceVersion(int64(kv.rev))
	}
	return nil
}

func (m *inmemory) put(resource string, scopes []store.Scope, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.NewInternalError(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := getkey(resource, scopes, name)
	kv, ok := m.kvs[key]
	if ok {
		return errors.NewAlreadyExists(resource, name)
	}
	kv.rev = m.rev.Add(1)
	kv.value = data
	m.kvs[key] = kv
	return nil
}

func (m *inmemory) delete(resource string, scopes []store.Scope, name string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := getkey(resource, scopes, name)
	kv, ok := m.kvs[key]
	if !ok {
		return errors.NewNotFound(resource, name)
	}
	if value != nil {
		if err := json.Unmarshal(kv.value, value); err != nil {
			return errors.NewInternalError(err)
		}
	}
	delete(m.kvs, key)
	return nil
}

func getkey(resource string, scopes []store.Scope, name string) string {
	key := "/" + resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	key += "/" + name
	return key
}

func (m *inmemory) on(ctx context.Context, into any, fn func(ctx context.Context, resources string) error) error {
	if into == nil {
		return errors.NewBadRequest("object is nil")
	}
	resources, err := store.GetResource(into)
	if err != nil {
		return err
	}
	return fn(ctx, resources)
}
