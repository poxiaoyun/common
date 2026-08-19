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

func (DynamicConfig) Set(context.Context, string, string, any, ...config.WriteOption) (*config.Configuration, error) {
	return nil, errors.NewUnsupported("noop dynamic config does not support writes")
}

func (DynamicConfig) Get(context.Context, string, string, any) (*config.Configuration, error) {
	return nil, nil
}

func (DynamicConfig) Patch(context.Context, string, string, config.Patch, any, ...config.WriteOption) (*config.Configuration, error) {
	return nil, errors.NewUnsupported("noop dynamic config does not support writes")
}

func (DynamicConfig) Watch(ctx context.Context, _, _ string) (config.Watcher, error) {
	watcher := &configurationWatcher{
		events:  make(chan config.Event),
		stopped: make(chan struct{}),
	}
	go watcher.run(ctx)
	return watcher, nil
}

type configurationWatcher struct {
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
	case w.events <- config.Event{Type: config.EventInitial}:
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
