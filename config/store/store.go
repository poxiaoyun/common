// Package store implements config.DynamicConfig with a common Store.
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"xiaoshiai.cn/common/config"
	commonerrors "xiaoshiai.cn/common/errors"
	commonstore "xiaoshiai.cn/common/store"
)

var _ config.DynamicConfig = (*DynamicConfig)(nil)

// AddToSchema registers Configuration persistence.
func AddToSchema(schema *commonstore.Schema) error {
	return schema.Register(&config.Configuration{}, commonstore.ResourceSchema{})
}

// New returns a Store-backed DynamicConfig that can access any namespace.
func New(storage commonstore.Store) *DynamicConfig {
	return &DynamicConfig{storage: storage}
}

// DynamicConfig stores configuration in a common Store.
type DynamicConfig struct {
	storage commonstore.Store
}

type writeOptions struct {
	ifAbsent        bool
	expectedVersion *int64
}

func resolveWriteOptions(options []config.WriteOption) (writeOptions, error) {
	resolved, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return writeOptions{}, err
	}
	return writeOptions{ifAbsent: resolved.IfAbsent, expectedVersion: resolved.ExpectedVersion}, nil
}

func (s *DynamicConfig) Set(ctx context.Context, namespace, name string, object any, options ...config.WriteOption) (*config.Configuration, error) {
	write, err := resolveWriteOptions(options)
	if err != nil {
		return nil, err
	}
	value, err := marshalObject(object)
	if err != nil {
		return nil, err
	}
	return s.setValue(ctx, namespace, name, value, write)
}

func (s *DynamicConfig) setValue(ctx context.Context, namespace, name string, value json.RawMessage, options writeOptions) (*config.Configuration, error) {
	storage := s.namespace(namespace)
	target := &config.Configuration{ObjectMeta: commonstore.ObjectMeta{ID: name, Name: name}, Value: value}
	if options.ifAbsent {
		if err := storage.Create(ctx, target); err != nil {
			return nil, err
		}
		return target, nil
	}
	if options.expectedVersion != nil {
		target.ResourceVersion = *options.expectedVersion
		if err := storage.Update(ctx, target); err != nil {
			return nil, err
		}
		return target, nil
	}

	for {
		target = &config.Configuration{ObjectMeta: commonstore.ObjectMeta{ID: name, Name: name}, Value: value}
		if err := storage.Update(ctx, target); err == nil {
			return target, nil
		} else if !commonerrors.IsNotFound(err) {
			return nil, err
		}
		target = &config.Configuration{ObjectMeta: commonstore.ObjectMeta{ID: name, Name: name}, Value: value}
		if err := storage.Create(ctx, target); err == nil {
			return target, nil
		} else if !commonerrors.IsAlreadyExists(err) {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

func (s *DynamicConfig) Get(ctx context.Context, namespace, name string, object any) (*config.Configuration, error) {
	if err := validateDecodeTarget(object); err != nil {
		return nil, err
	}
	configuration, err := s.getValue(ctx, namespace, name)
	if commonerrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalObject(configuration.Value, object); err != nil {
		return nil, err
	}
	return configuration, nil
}

func (s *DynamicConfig) getValue(ctx context.Context, namespace, name string) (*config.Configuration, error) {
	configuration := &config.Configuration{}
	if err := s.namespace(namespace).Get(ctx, name, configuration); err != nil {
		return nil, err
	}
	return configuration, nil
}

func (s *DynamicConfig) Patch(ctx context.Context, namespace, name string, patch config.Patch, object any, options ...config.WriteOption) (*config.Configuration, error) {
	write, err := resolveWriteOptions(options)
	if err != nil {
		return nil, err
	}
	if write.ifAbsent {
		return nil, commonerrors.NewBadRequest("IfAbsent is not valid for Patch")
	}
	if object != nil {
		if err := validateDecodeTarget(object); err != nil {
			return nil, err
		}
	}
	if !json.Valid(patch.Data) {
		return nil, commonerrors.NewBadRequest("configuration patch must be valid JSON")
	}

	var configuration *config.Configuration
	switch patch.Type {
	case config.MergePatch:
		configuration, err = s.applyMergePatch(ctx, namespace, name, patch.Data, write)
	case config.JSONPatch:
		configuration, err = s.applyJSONPatch(ctx, namespace, name, patch.Data, write)
	default:
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("unsupported configuration patch type %q", patch.Type))
	}
	if err != nil {
		return nil, err
	}
	if object != nil {
		if err := unmarshalObject(configuration.Value, object); err != nil {
			return nil, err
		}
	}
	return configuration, nil
}

func (s *DynamicConfig) applyMergePatch(ctx context.Context, namespace, name string, data json.RawMessage, options writeOptions) (*config.Configuration, error) {
	wrapped, err := json.Marshal(struct {
		Value           json.RawMessage `json:"value"`
		ResourceVersion *int64          `json:"resourceVersion,omitempty"`
	}{Value: data, ResourceVersion: options.expectedVersion})
	if err != nil {
		return nil, err
	}
	return s.patchValue(ctx, namespace, name, commonstore.RawPatch(commonstore.PatchTypeMergePatch, wrapped))
}

func (s *DynamicConfig) applyJSONPatch(ctx context.Context, namespace, name string, data json.RawMessage, options writeOptions) (*config.Configuration, error) {
	if options.expectedVersion != nil {
		current, err := s.getValue(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		if current.ResourceVersion != *options.expectedVersion {
			return nil, commonerrors.NewConflict("configuration", name,
				fmt.Errorf("resourceVersion %d does not match", *options.expectedVersion))
		}
		if err := commonstore.JsonPatchObject(&current.Value, data); err != nil {
			return nil, err
		}
		return s.setValue(ctx, namespace, name, current.Value, options)
	}
	wrapped, err := configurationJSONPatch(data)
	if err != nil {
		return nil, err
	}
	return s.patchValue(ctx, namespace, name, commonstore.RawPatch(commonstore.PatchTypeJSONPatch, wrapped))
}

func (s *DynamicConfig) patchValue(ctx context.Context, namespace, name string, patch commonstore.Patch) (*config.Configuration, error) {
	target := &config.Configuration{ObjectMeta: commonstore.ObjectMeta{ID: name}}
	if err := s.namespace(namespace).Patch(ctx, target, patch); err != nil {
		return nil, err
	}
	return target, nil
}

type jsonPatchOperation struct {
	Operation string          `json:"op"`
	Path      string          `json:"path"`
	From      *string         `json:"from,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

func configurationJSONPatch(data json.RawMessage) ([]byte, error) {
	operations := []jsonPatchOperation{}
	if err := json.Unmarshal(data, &operations); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	for index := range operations {
		operations[index].Path = "/value" + operations[index].Path
		if operations[index].From != nil {
			from := "/value" + *operations[index].From
			operations[index].From = &from
		}
	}
	return json.Marshal(operations)
}

func (s *DynamicConfig) Watch(ctx context.Context, namespace, name string) (config.Watcher, error) {
	upstream, err := s.namespace(namespace).Watch(ctx, &commonstore.List[config.Configuration]{},
		commonstore.WithID(name), commonstore.WithSendInitialEvents())
	if err != nil {
		return nil, err
	}
	watcher := &configurationWatcher{
		upstream: upstream,
		events:   make(chan config.Event),
		stopped:  make(chan struct{}),
	}
	go watcher.run(ctx)
	return watcher, nil
}

type configurationWatcher struct {
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
	var initial *config.Configuration
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
					configuration, err := watchConfiguration(event.Object)
					if err != nil {
						w.send(ctx, config.Event{Error: err})
						return
					}
					initial = configuration
				case commonstore.WatchEventDelete:
					initial = nil
				case commonstore.WatchEventBookmark:
					if !w.send(ctx, config.Event{Type: config.EventInitial, Configuration: initial}) {
						return
					}
					initialized = true
				}
				continue
			}

			switch event.Type {
			case commonstore.WatchEventCreate, commonstore.WatchEventUpdate:
				configuration, err := watchConfiguration(event.Object)
				if err != nil {
					w.send(ctx, config.Event{Error: err})
					return
				}
				if !w.send(ctx, config.Event{Type: config.EventChange, Configuration: configuration}) {
					return
				}
			case commonstore.WatchEventDelete:
				if !w.send(ctx, config.Event{Type: config.EventDelete}) {
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

func watchConfiguration(object any) (*config.Configuration, error) {
	configuration, ok := object.(*config.Configuration)
	if !ok {
		return nil, fmt.Errorf("configuration watcher returned %T", object)
	}
	copy := *configuration
	copy.Value = bytes.Clone(configuration.Value)
	return &copy, nil
}

func (s *DynamicConfig) namespace(namespace string) commonstore.Store {
	return s.storage.Scope(commonstore.Scope{Resource: "namespaces", Name: namespace})
}

func marshalObject(object any) (json.RawMessage, error) {
	value, err := json.Marshal(object)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("marshal configuration object: %v", err))
	}
	return value, nil
}

func unmarshalObject(value json.RawMessage, object any) error {
	if err := validateDecodeTarget(object); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(object); err != nil {
		return fmt.Errorf("decode configuration object: %w", err)
	}
	return nil
}

func validateDecodeTarget(object any) error {
	if object == nil {
		return commonerrors.NewBadRequest("configuration object must be a non-nil pointer")
	}
	value := reflect.ValueOf(object)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return commonerrors.NewBadRequest("configuration object must be a non-nil pointer")
	}
	return nil
}
