package config

import (
	"context"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

type DynamicConfig interface {
	// Get get the value of the key.
	// if the key is not exist, return an empty string.
	Get(ctx context.Context, key string) (string, error)
	// Set set the value of the key.
	Set(ctx context.Context, key, value string) error

	// AutoUpdate will update the value passed in config object when the config changed.
	// it use the key to watch the config change event.
	// default using json to marshal the config object.
	AutoUpdate(ctx context.Context, key string, config any) error
}

func NewStoreDynamicConfig(store store.Store) DynamicConfig {
	return &StoreDynamicConfig{store: store}
}

type StoreDynamicConfig struct {
	store store.Store
}

type Setting struct {
	store.ObjectMeta `json:",inline"`
	Value            string `json:"value"`
}

// AutoUpdate implements DynamicConfig.
func (s *StoreDynamicConfig) AutoUpdate(ctx context.Context, key string, config any) error {
	if _, err := store.EnforcePtr(config); err != nil {
		return err
	}
	return errors.NewNotImplemented("auto update is not implemented")
}

// Get implements DynamicConfig.
func (s *StoreDynamicConfig) Get(ctx context.Context, key string) (string, error) {
	setting := &Setting{}
	if err := s.store.Get(ctx, key, setting); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

// Set implements DynamicConfig.
func (s *StoreDynamicConfig) Set(ctx context.Context, key string, value string) error {
	setting := &Setting{
		ObjectMeta: store.ObjectMeta{Name: key},
	}
	return store.CreateOrUpdate(ctx, s.store, setting, func() error {
		setting.Value = value
		return nil
	})
}

var _ DynamicConfig = &StoreDynamicConfig{}
