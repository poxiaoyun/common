package config

import (
	"context"

	"xiaoshiai.cn/common/controller"
	"xiaoshiai.cn/common/store"
)

type DynamicConfig interface {
	Set(ctx context.Context, key string, val string) error
	Get(ctx context.Context, key string) (string, error)
	List(ctx context.Context) (map[string]string, error)
	Watch(ctx context.Context, onChanged func(ctx context.Context, key string, val string)) error
}

type DynamicConfigOptions struct {
	// Server is the server address for the dynamic configuration service.
	Server string `json:"server" description:"Server address for the dynamic configuration service, e.g., 'http://localhost:8080'"`
	// Token is the authentication token for accessing the dynamic configuration service.
	Token string `json:"token" description:"Authentication token for the dynamic configuration service, used for secure access"`
	// Component is the name of the component that this config belongs to.
	// It is used to distinguish configs for different components in the same configuration store.
	Component string `json:"component" description:"Name of the component that this config belongs to, used for distinguishing configs in the same store"`
}

func NewDefaultDynamicConfigOptions(component string) *DynamicConfigOptions {
	return &DynamicConfigOptions{
		Server:    "http://config-server:8080",
		Token:     "",
		Component: component,
	}
}

func NewStoreDynamicConfig(storage store.Store, options *DynamicConfigOptions) DynamicConfig {
	storage = storage.Scope(store.Scope{Resource: "configs", Name: options.Component})
	return &StoreDynamicConfig{Storage: storage}
}

type Setting struct {
	store.ObjectMeta `json:",inline"`
	Value            string `json:"value"`
}

type StoreDynamicConfig struct {
	Storage store.Store
}

// List implements DynamicConfigUpdater.
func (s *StoreDynamicConfig) List(ctx context.Context) (map[string]string, error) {
	settings := &store.List[Setting]{}
	if err := s.Storage.List(ctx, settings); err != nil {
		return nil, store.IgnoreNotFound(err)
	}
	result := make(map[string]string, len(settings.Items))
	for _, setting := range settings.Items {
		result[setting.Name] = setting.Value
	}
	return result, nil
}

// Get implements DynamicConfigUpdater.
func (s *StoreDynamicConfig) Get(ctx context.Context, key string) (string, error) {
	setting := &Setting{}
	if err := s.Storage.Get(ctx, key, setting); err != nil {
		return "", store.IgnoreNotFound(err)
	}
	return setting.Value, nil
}

// Set implements DynamicConfigUpdater.
func (s *StoreDynamicConfig) Set(ctx context.Context, key string, val string) error {
	setting := &Setting{
		ObjectMeta: store.ObjectMeta{Name: key},
		Value:      val,
	}
	return store.CreateOrUpdate(ctx, s.Storage, setting, func() error {
		setting.Value = val
		return nil
	})
}

// Watch implements DynamicConfigUpdater.
func (s *StoreDynamicConfig) Watch(ctx context.Context, onChanged func(ctx context.Context, key string, val string)) error {
	handler := func(ctx context.Context, kind store.WatchEventType, obj *Setting) error {
		switch kind {
		case store.WatchEventCreate, store.WatchEventUpdate:
			if obj == nil {
				return nil
			}
			onChanged(ctx, obj.Name, obj.Value)
		case store.WatchEventDelete:
			onChanged(ctx, obj.Name, "")
		}
		return nil
	}
	return controller.RunTypedListWatchContext(ctx, s.Storage,
		controller.EventHandlerFunc[*Setting](handler),
		store.WithSendInitialEvents())
}
