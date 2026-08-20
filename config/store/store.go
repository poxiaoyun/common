// Package store implements config.DynamicConfig with a common Store.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"xiaoshiai.cn/common/config"
	commonerrors "xiaoshiai.cn/common/errors"
	commonstore "xiaoshiai.cn/common/store"
)

var _ config.DynamicConfig = (*DynamicConfig)(nil)

// StoredConfiguration is the Store persistence representation of a Configuration.
type StoredConfiguration struct {
	commonstore.ObjectMeta `json:",inline"`
	Value                  config.Object `json:"value"`
}

// ResourceName returns the stable Store resource name.
func (*StoredConfiguration) ResourceName() string {
	return "configurations"
}

// AddToSchema registers StoredConfiguration persistence.
func AddToSchema(schema *commonstore.Schema) error {
	return schema.Register(&StoredConfiguration{}, commonstore.ResourceSchema{})
}

// New returns a Store-backed DynamicConfig that can access any namespace.
func New(storage commonstore.Store) *DynamicConfig {
	return &DynamicConfig{storage: storage}
}

// DynamicConfig stores configuration in a common Store.
type DynamicConfig struct {
	storage commonstore.Store
}

func (s *DynamicConfig) Set(ctx context.Context, namespace, name string, object any, options ...config.WriteOption) (config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return config.Configuration{}, err
	}
	value, err := config.EncodeObject(object)
	if err != nil {
		return config.Configuration{}, err
	}
	if write.ExpectedVersion != nil {
		return s.setConfigurationVersion(ctx, namespace, name, value, *write.ExpectedVersion)
	}
	return s.setConfiguration(ctx, namespace, name, value)
}

func (s *DynamicConfig) setConfiguration(ctx context.Context, namespace, name string, value config.Object) (config.Configuration, error) {
	storage := s.namespace(namespace)
	for {
		target, err := s.getStoredConfiguration(ctx, namespace, name)
		if err != nil {
			if !commonerrors.IsNotFound(err) {
				return config.Configuration{}, err
			}
			target = newStoredConfiguration(name, value)
			err = storage.Create(ctx, target)
			if err != nil {
				if !commonerrors.IsAlreadyExists(err) {
					return config.Configuration{}, err
				}
				if err := ctx.Err(); err != nil {
					return config.Configuration{}, err
				}
				continue
			}
			return configurationFromStored(target)
		}

		target.Value = value
		target.ResourceVersion = 0
		err = storage.Update(ctx, target)
		if err != nil {
			if !commonerrors.IsNotFound(err) {
				return config.Configuration{}, err
			}
			if err := ctx.Err(); err != nil {
				return config.Configuration{}, err
			}
			continue
		}
		return configurationFromStored(target)
	}
}

func (s *DynamicConfig) setConfigurationVersion(ctx context.Context, namespace, name string, value config.Object, version int64) (config.Configuration, error) {
	target := newStoredConfiguration(name, value)
	if version == 0 {
		err := s.namespace(namespace).Create(ctx, target)
		if err != nil {
			if commonerrors.IsAlreadyExists(err) {
				return config.Configuration{}, versionConflict(name, version)
			}
			return config.Configuration{}, err
		}
		return configurationFromStored(target)
	}
	target.ResourceVersion = version
	err := s.namespace(namespace).Update(ctx, target)
	if err != nil {
		if commonerrors.IsNotFound(err) {
			return config.Configuration{}, versionConflict(name, version)
		}
		return config.Configuration{}, err
	}
	return configurationFromStored(target)
}

func (s *DynamicConfig) Get(ctx context.Context, namespace, name string, object any) (config.Configuration, error) {
	stored, err := s.getStoredConfiguration(ctx, namespace, name)
	if err != nil {
		if !commonerrors.IsNotFound(err) {
			return config.Configuration{}, err
		}
		result := emptyConfiguration(name)
		if err := result.Value.Decode(object); err != nil {
			return config.Configuration{}, err
		}
		return result, nil
	}
	result, err := configurationFromStored(stored)
	if err != nil {
		return config.Configuration{}, err
	}
	if err := result.Value.Decode(object); err != nil {
		return config.Configuration{}, err
	}
	return result, nil
}

func (s *DynamicConfig) Patch(ctx context.Context, namespace, name string, patch config.Patch, object any, options ...config.WriteOption) (config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return config.Configuration{}, err
	}
	if patch.Type != config.MergePatch && patch.Type != config.JSONPatch {
		return config.Configuration{}, commonerrors.NewBadRequest(fmt.Sprintf("unsupported configuration patch type %q", patch.Type))
	}
	if !json.Valid(patch.Data) {
		return config.Configuration{}, commonerrors.NewBadRequest("configuration patch must be valid JSON")
	}
	result, err := s.patchConfiguration(ctx, namespace, name, patch, write.ExpectedVersion)
	if err != nil {
		return config.Configuration{}, err
	}
	if object != nil {
		if err := result.Value.Decode(object); err != nil {
			return config.Configuration{}, err
		}
	}
	return result, nil
}

func (s *DynamicConfig) patchConfiguration(ctx context.Context, namespace, name string, patch config.Patch, expectedVersion *int64) (config.Configuration, error) {
	storage := s.namespace(namespace)
	for {
		current, err := s.getStoredConfiguration(ctx, namespace, name)
		if err != nil {
			if !commonerrors.IsNotFound(err) {
				return config.Configuration{}, err
			}
			if expectedVersion != nil && *expectedVersion > 0 {
				return config.Configuration{}, versionConflict(name, *expectedVersion)
			}
			value, err := applyPatch(config.Object{}, patch)
			if err != nil {
				return config.Configuration{}, err
			}
			target := newStoredConfiguration(name, value)
			err = storage.Create(ctx, target)
			if err != nil {
				if commonerrors.IsAlreadyExists(err) {
					if expectedVersion != nil {
						return config.Configuration{}, versionConflict(name, *expectedVersion)
					}
					continue
				}
				return config.Configuration{}, err
			}
			return configurationFromStored(target)
		}

		if expectedVersion != nil && current.ResourceVersion != *expectedVersion {
			return config.Configuration{}, versionConflict(name, *expectedVersion)
		}
		value, err := applyPatch(current.Value, patch)
		if err != nil {
			return config.Configuration{}, err
		}
		current.Value = value
		err = storage.Update(ctx, current)
		if err != nil {
			if expectedVersion == nil && (commonerrors.IsConflict(err) || commonerrors.IsNotFound(err)) {
				continue
			}
			if commonerrors.IsNotFound(err) {
				return config.Configuration{}, versionConflict(name, *expectedVersion)
			}
			return config.Configuration{}, err
		}
		return configurationFromStored(current)
	}
}

func applyPatch(current config.Object, patch config.Patch) (config.Object, error) {
	next, err := config.EncodeObject(current)
	if err != nil {
		return nil, err
	}
	switch patch.Type {
	case config.MergePatch:
		err = commonstore.JsonMergePatchObject(&next, patch.Data)
	case config.JSONPatch:
		err = commonstore.JsonPatchObject(&next, patch.Data)
	}
	if err != nil {
		return nil, err
	}
	return config.EncodeObject(next)
}

// ListKeys returns the namespace's keys in stable name order.
func (s *DynamicConfig) ListKeys(ctx context.Context, namespace string) ([]config.Key, error) {
	list := &commonstore.List[StoredConfiguration]{}
	storage := s.namespace(namespace)
	if err := storage.List(ctx, list, commonstore.WithSort("id+")); err != nil {
		return nil, err
	}
	keys := make([]config.Key, 0, len(list.Items))
	for _, item := range list.Items {
		keys = append(keys, config.Key{Name: item.Name, Version: item.ResourceVersion})
	}
	return keys, nil
}

func (s *DynamicConfig) Watch(ctx context.Context, namespace, name string) (config.Watcher, error) {
	storage := s.namespace(namespace)
	upstream, err := storage.Watch(ctx, &commonstore.List[StoredConfiguration]{},
		commonstore.WithID(name), commonstore.WithSendInitialEvents())
	if err != nil {
		return nil, err
	}
	watcher := &configurationWatcher{
		name:     name,
		upstream: upstream,
		events:   make(chan config.Event),
		stopped:  make(chan struct{}),
	}
	go watcher.run(ctx)
	return watcher, nil
}

type configurationWatcher struct {
	name     string
	upstream commonstore.Watcher
	events   chan config.Event
	stopped  chan struct{}
	stopOnce sync.Once
}

func (w *configurationWatcher) Events() <-chan config.Event {
	return w.events
}

func (w *configurationWatcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopped)
		w.upstream.Stop()
	})
}

func (w *configurationWatcher) run(ctx context.Context) {
	defer close(w.events)
	defer w.Stop()
	initial := emptyConfiguration(w.name)
	initialized := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopped:
			return
		case event, open := <-w.upstream.Events():
			if !open {
				if !w.wasStopped() && ctx.Err() == nil {
					w.send(ctx, config.Event{Error: fmt.Errorf("configuration watcher closed")})
				}
				return
			}
			if event.Error != nil {
				w.send(ctx, config.Event{Error: event.Error})
				return
			}
			if !initialized {
				switch event.Type {
				case commonstore.WatchEventCreate, commonstore.WatchEventUpdate:
					current, err := watchConfiguration(event.Object)
					if err != nil {
						w.send(ctx, config.Event{Error: err})
						return
					}
					initial = current
				case commonstore.WatchEventDelete:
					initial = emptyConfiguration(w.name)
				case commonstore.WatchEventBookmark:
					if !w.send(ctx, config.Event{Configuration: initial}) {
						return
					}
					initialized = true
				}
				continue
			}

			switch event.Type {
			case commonstore.WatchEventCreate, commonstore.WatchEventUpdate:
				current, err := watchConfiguration(event.Object)
				if err != nil {
					w.send(ctx, config.Event{Error: err})
					return
				}
				if !w.send(ctx, config.Event{Configuration: current}) {
					return
				}
			case commonstore.WatchEventDelete:
				if !w.send(ctx, config.Event{Configuration: emptyConfiguration(w.name)}) {
					return
				}
			}
		}
	}
}

func (w *configurationWatcher) send(ctx context.Context, event config.Event) bool {
	select {
	case w.events <- event:
		return true
	case <-ctx.Done():
		return false
	case <-w.stopped:
		return false
	}
}

func (w *configurationWatcher) wasStopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func (s *DynamicConfig) getStoredConfiguration(ctx context.Context, namespace, name string) (*StoredConfiguration, error) {
	result := &StoredConfiguration{}
	storage := s.namespace(namespace)
	if err := storage.Get(ctx, name, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *DynamicConfig) namespace(namespace string) commonstore.Store {
	return s.storage.Scope(commonstore.Scope{Resource: "namespaces", Name: namespace})
}

func newStoredConfiguration(name string, value config.Object) *StoredConfiguration {
	return &StoredConfiguration{ObjectMeta: commonstore.ObjectMeta{ID: name, Name: name}, Value: value}
}

func configurationFromStored(stored *StoredConfiguration) (config.Configuration, error) {
	value, err := config.EncodeObject(stored.Value)
	if err != nil {
		return config.Configuration{}, err
	}
	return config.Configuration{Name: stored.Name, Version: stored.ResourceVersion, Value: value}, nil
}

func watchConfiguration(object any) (config.Configuration, error) {
	stored, ok := object.(*StoredConfiguration)
	if !ok {
		return config.Configuration{}, fmt.Errorf("configuration watcher returned %T", object)
	}
	return configurationFromStored(stored)
}

func emptyConfiguration(name string) config.Configuration {
	return config.Configuration{Name: name, Value: config.Object{}}
}

func versionConflict(name string, version int64) error {
	return commonerrors.NewConflict("configuration", name, fmt.Errorf("version %d does not match", version))
}
