// Package noop implements a disabled config.DynamicConfig.
package noop

import (
	"context"
	"sync"

	"xiaoshiai.cn/common/config"
	"xiaoshiai.cn/common/errors"
)

var _ config.DynamicConfig = DynamicConfig{}

// DynamicConfig represents a configuration center that is not enabled.
type DynamicConfig struct{}

// New returns a disabled DynamicConfig.
func New() DynamicConfig {
	return DynamicConfig{}
}

func (DynamicConfig) Set(context.Context, string, string, any, ...config.WriteOption) (config.Configuration, error) {
	return config.Configuration{}, errors.NewUnsupported("noop dynamic config does not support writes")
}

func (DynamicConfig) Get(_ context.Context, _, name string, object any) (config.Configuration, error) {
	result := emptyConfiguration(name)
	if err := result.Value.Decode(object); err != nil {
		return config.Configuration{}, err
	}
	return result, nil
}

func (DynamicConfig) Patch(context.Context, string, string, config.Patch, any, ...config.WriteOption) (config.Configuration, error) {
	return config.Configuration{}, errors.NewUnsupported("noop dynamic config does not support writes")
}

// ListKeys returns no keys for the disabled configuration center.
func (DynamicConfig) ListKeys(context.Context, string) ([]config.Key, error) {
	return []config.Key{}, nil
}

func (DynamicConfig) Watch(ctx context.Context, _, name string) (config.Watcher, error) {
	watcher := &configurationWatcher{
		name:    name,
		events:  make(chan config.Event),
		stopped: make(chan struct{}),
	}
	go watcher.run(ctx)
	return watcher, nil
}

type configurationWatcher struct {
	name     string
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
	})
}

func (w *configurationWatcher) run(ctx context.Context) {
	defer close(w.events)
	select {
	case w.events <- config.Event{Configuration: emptyConfiguration(w.name)}:
	case <-ctx.Done():
		return
	case <-w.stopped:
		return
	}
	select {
	case <-ctx.Done():
	case <-w.stopped:
	}
}

func emptyConfiguration(name string) config.Configuration {
	return config.Configuration{Name: name, Value: config.Object{}}
}
