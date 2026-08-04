package inmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

var _ store.PingableStore = &InMemory{}

type InMemory struct {
	core   *inmemory
	scopes []store.Scope
	status bool
}

func New(scheme *store.Schema) (*InMemory, error) {
	scheme, err := scheme.Clone()
	if err != nil {
		return nil, err
	}
	return &InMemory{core: &inmemory{
		kvs:     map[string]kv{},
		indexes: map[string]map[string]map[string]map[string]struct{}{},
		schema:  scheme,
	}}, nil
}

// Ping implements store.Pinger. An in-memory store has no external backend to
// probe, so it is always available while the process is running.
func (i *InMemory) Ping(context.Context) error {
	return nil
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
		return i.core.create(resources, i.scopes, obj.GetID(), obj)
	})
}

func (i *InMemory) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.delete(resources, i.scopes, obj.GetID(), nil)
	})
}

func (i *InMemory) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	return errors.NewNotImplemented("delete batch is not supported")
}

func (i *InMemory) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	return i.core.on(ctx, obj, func(ctx context.Context, resources string) error {
		return i.core.get(resources, i.scopes, name, obj)
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
		return i.core.put(resources, i.scopes, obj.GetID(), obj)
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
	mu      sync.RWMutex
	rev     atomic.Uint64
	kvs     map[string]kv
	indexes map[string]map[string]map[string]map[string]struct{}
	schema  *store.Schema
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

	indexValues, err := m.indexValues(resource, scopes, data)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := getkey(resource, scopes, name)
	if _, ok := m.kvs[key]; ok {
		return errors.NewAlreadyExists(resource, name)
	}
	if err := m.checkUnique(resource, key, indexValues); err != nil {
		return err
	}
	m.kvs[key] = kv{
		value: data,
		rev:   m.rev.Add(1),
	}
	m.addIndexes(resource, key, indexValues)
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
	indexValues, err := m.indexValues(resource, scopes, data)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := getkey(resource, scopes, name)
	kv, ok := m.kvs[key]
	if !ok {
		return errors.NewNotFound(resource, name)
	}
	if err := m.checkUnique(resource, key, indexValues); err != nil {
		return err
	}
	oldIndexValues, err := m.indexValues(resource, scopes, kv.value)
	if err != nil {
		return err
	}
	m.removeIndexes(resource, key, oldIndexValues)
	kv.rev = m.rev.Add(1)
	kv.value = data
	m.kvs[key] = kv
	m.addIndexes(resource, key, indexValues)
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
	indexValues, err := m.indexValues(resource, scopes, kv.value)
	if err != nil {
		return err
	}
	m.removeIndexes(resource, key, indexValues)
	delete(m.kvs, key)
	return nil
}

func (m *inmemory) indexValues(resource string, scopes []store.Scope, data []byte) (map[string]string, error) {
	definition, err := m.schema.Resource(resource)
	if err != nil {
		return nil, err
	}
	object := map[string]any{}
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, errors.NewInternalError(err)
	}
	values := make(map[string]string, len(definition.Indexes))
	for _, index := range definition.Indexes {
		parts := make([]any, 0, len(index.Fields))
		skip := false
		for _, field := range index.Fields {
			var value any
			found := false
			if scopeIndex := slices.Index(definition.ScopeKeys, field); scopeIndex >= 0 {
				if scopeIndex < len(scopes) {
					value, found = scopes[scopeIndex].Name, true
				}
			} else {
				value, found = store.GetNestedField(object, strings.Split(field, ".")...)
			}
			if (!found || value == nil) && index.Nullable {
				skip = true
				break
			}
			parts = append(parts, value)
		}
		if skip {
			continue
		}
		encoded, err := json.Marshal(parts)
		if err != nil {
			return nil, errors.NewInternalError(err)
		}
		values[index.Name] = string(encoded)
	}
	return values, nil
}

func (m *inmemory) checkUnique(resource, objectKey string, values map[string]string) error {
	definition, err := m.schema.Resource(resource)
	if err != nil {
		return err
	}
	for _, index := range definition.Indexes {
		if !index.Unique {
			continue
		}
		value, exists := values[index.Name]
		if !exists {
			continue
		}
		for key := range m.indexEntries(resource, index.Name, value) {
			if key != objectKey {
				return errors.NewAlreadyExists(resource, fmt.Sprintf("index %s", index.Name))
			}
		}
	}
	return nil
}

func (m *inmemory) indexEntries(resource, indexName, value string) map[string]struct{} {
	resourceIndexes := m.indexes[resource]
	if resourceIndexes == nil {
		resourceIndexes = map[string]map[string]map[string]struct{}{}
		m.indexes[resource] = resourceIndexes
	}
	index := resourceIndexes[indexName]
	if index == nil {
		index = map[string]map[string]struct{}{}
		resourceIndexes[indexName] = index
	}
	entries := index[value]
	if entries == nil {
		entries = map[string]struct{}{}
		index[value] = entries
	}
	return entries
}

func (m *inmemory) addIndexes(resource, objectKey string, values map[string]string) {
	for indexName, value := range values {
		m.indexEntries(resource, indexName, value)[objectKey] = struct{}{}
	}
}

func (m *inmemory) removeIndexes(resource, objectKey string, values map[string]string) {
	for indexName, value := range values {
		entries := m.indexEntries(resource, indexName, value)
		delete(entries, objectKey)
		if len(entries) == 0 {
			delete(m.indexes[resource][indexName], value)
		}
	}
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
