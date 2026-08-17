package config

import (
	"context"
	"fmt"
	"slices"

	"xiaoshiai.cn/common/controller"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

// Entry is one versioned dynamic configuration value.
type Entry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// SetOptions controls the write precondition for one configuration value.
type SetOptions struct {
	// ExpectedVersion nil performs an upsert, zero creates only when absent,
	// and a positive value performs a compare-and-swap update.
	ExpectedVersion *int64
}

// DeleteOptions controls the delete precondition for one configuration value.
type DeleteOptions struct {
	// ExpectedVersion nil deletes the current value. A positive value requires
	// that exact version.
	ExpectedVersion *int64
}

// Event describes one ordered dynamic configuration change.
type Event struct {
	Type  store.WatchEventType
	Entry Entry
}

// DynamicConfig stores versioned configuration values.
type DynamicConfig interface {
	// Set writes key according to options and returns the persisted version.
	Set(ctx context.Context, key, value string, options SetOptions) (Entry, error)
	// Get returns the current value and version for key.
	Get(ctx context.Context, key string) (Entry, error)
	// List returns all values ordered by key.
	List(ctx context.Context) ([]Entry, error)
	// Delete removes key according to options.
	Delete(ctx context.Context, key string, options DeleteOptions) error
	// Watch observes ordered configuration changes.
	Watch(ctx context.Context, onChanged func(context.Context, Event) error) error
}

// DynamicConfigOptions identifies one component's configuration scope.
type DynamicConfigOptions struct {
	Server    string `json:"server" description:"Server address for the dynamic configuration service, e.g., 'http://localhost:8080'"`
	Token     string `json:"token" description:"Authentication token for accessing the dynamic configuration service"`
	Component string `json:"component" description:"Component name used to isolate configuration values"`
}

// NewDefaultDynamicConfigOptions returns the default centralized configuration options.
func NewDefaultDynamicConfigOptions(component string) *DynamicConfigOptions {
	return &DynamicConfigOptions{Server: "http://config-server:8080", Component: component}
}

// AddToStoreSchema registers the dynamic configuration resource.
func AddToStoreSchema(schema *store.Schema) error {
	return schema.Register(&Setting{}, store.ResourceSchema{})
}

// NewStoreDynamicConfig returns a Store-backed dynamic configuration adapter.
func NewStoreDynamicConfig(storage store.Store, options *DynamicConfigOptions) DynamicConfig {
	storage = storage.Scope(store.Scope{Resource: "configs", Name: options.Component})
	return &StoreDynamicConfig{Storage: storage}
}

// Setting is the Store representation of one dynamic configuration value.
type Setting struct {
	store.ObjectMeta `json:",inline"`
	Value            string `json:"value"`
}

// StoreDynamicConfig stores configuration values in a common Store.
type StoreDynamicConfig struct {
	Storage store.Store
}

// List returns all values ordered by key.
func (s *StoreDynamicConfig) List(ctx context.Context) ([]Entry, error) {
	settings := &store.List[Setting]{}
	if err := s.Storage.List(ctx, settings); err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(settings.Items))
	for _, setting := range settings.Items {
		result = append(result, EntryFromSetting(&setting))
	}
	slices.SortFunc(result, func(left, right Entry) int {
		if left.Key < right.Key {
			return -1
		}
		if left.Key > right.Key {
			return 1
		}
		return 0
	})
	return result, nil
}

// Get returns the current value and version for key.
func (s *StoreDynamicConfig) Get(ctx context.Context, key string) (Entry, error) {
	setting := &Setting{}
	if err := s.Storage.Get(ctx, key, setting); err != nil {
		return Entry{}, err
	}
	return EntryFromSetting(setting), nil
}

// Set writes key according to options and returns the persisted version.
func (s *StoreDynamicConfig) Set(ctx context.Context, key, value string, options SetOptions) (Entry, error) {
	setting := &Setting{ObjectMeta: store.ObjectMeta{ID: key, Name: key}, Value: value}
	if options.ExpectedVersion != nil {
		if *options.ExpectedVersion == 0 {
			if err := s.Storage.Create(ctx, setting); err != nil {
				return Entry{}, err
			}
			return EntryFromSetting(setting), nil
		}
		setting.ResourceVersion = *options.ExpectedVersion
		if err := s.Storage.Update(ctx, setting); err != nil {
			return Entry{}, err
		}
		return EntryFromSetting(setting), nil
	}
	current := &Setting{}
	err := s.Storage.Get(ctx, key, current)
	if err != nil {
		if !commonerrors.IsNotFound(err) {
			return Entry{}, err
		}
		if err := s.Storage.Create(ctx, setting); err != nil {
			return Entry{}, err
		}
		return EntryFromSetting(setting), nil
	}
	setting.ResourceVersion = current.ResourceVersion
	if err := s.Storage.Update(ctx, setting); err != nil {
		return Entry{}, err
	}
	return EntryFromSetting(setting), nil
}

// Delete removes key according to options.
func (s *StoreDynamicConfig) Delete(ctx context.Context, key string, options DeleteOptions) error {
	setting := &Setting{}
	if err := s.Storage.Get(ctx, key, setting); err != nil {
		return err
	}
	if options.ExpectedVersion != nil && setting.ResourceVersion != *options.ExpectedVersion {
		return commonerrors.NewConflict("setting", key, fmt.Errorf("resourceVersion %d does not match", *options.ExpectedVersion))
	}
	if options.ExpectedVersion != nil {
		return s.Storage.Delete(ctx, setting, store.WithDeleteFieldRequirements(
			store.RequirementEqual("resourceVersion", *options.ExpectedVersion),
		))
	}
	return s.Storage.Delete(ctx, setting)
}

// Watch observes ordered configuration changes.
func (s *StoreDynamicConfig) Watch(ctx context.Context, onChanged func(context.Context, Event) error) error {
	handler := func(ctx context.Context, event controller.TypedWatchEvent[*Setting]) error {
		if event.Type == store.WatchEventBookmark {
			return nil
		}
		return onChanged(ctx, Event{Type: event.Type, Entry: EntryFromSetting(event.Object)})
	}
	return controller.RunTypedListWatchContext(
		ctx,
		s.Storage,
		controller.EventHandlerFunc[*Setting](handler),
		store.WithSendInitialEvents(),
	)
}

// EntryFromSetting converts a persisted Setting to its public value.
func EntryFromSetting(setting *Setting) Entry {
	return Entry{Key: setting.ID, Value: setting.Value, Version: setting.ResourceVersion}
}
