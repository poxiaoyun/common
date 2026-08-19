// Package http implements config.DynamicConfig over IAM's HTTP projection.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"

	"xiaoshiai.cn/common/config"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

var _ config.DynamicConfig = (*DynamicConfig)(nil)

// New returns an HTTP-backed DynamicConfig.
func New(ctx context.Context, address, token string) (*DynamicConfig, error) {
	client, err := httpclient.NewClientFromConfig(ctx, &httpclient.Config{Server: address, Token: token})
	if err != nil {
		return nil, err
	}
	return &DynamicConfig{client: client}, nil
}

// DynamicConfig stores configuration through the HTTP projection.
type DynamicConfig struct {
	client *httpclient.Client
}

func (c *DynamicConfig) Set(ctx context.Context, namespace, name string, object any, options ...config.WriteOption) (*config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return nil, err
	}
	value, err := marshalObject(object)
	if err != nil {
		return nil, err
	}
	request := c.client.Put(c.itemPath(namespace, name)).JSON(struct {
		Value json.RawMessage `json:"value"`
	}{Value: value})
	setWritePreconditionHeaders(request, write.IfAbsent, write.ExpectedVersion)
	result := &config.Configuration{}
	if _, err := request.Return(result).Do(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *DynamicConfig) Get(ctx context.Context, namespace, name string, object any) (*config.Configuration, error) {
	if err := validateDecodeTarget(object); err != nil {
		return nil, err
	}
	result := &config.Configuration{}
	if _, err := c.client.Get(c.itemPath(namespace, name)).Return(result).Do(ctx); err != nil {
		if commonerrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := unmarshalObject(result.Value, object); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *DynamicConfig) Patch(ctx context.Context, namespace, name string, patch config.Patch, object any, options ...config.WriteOption) (*config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return nil, err
	}
	if write.IfAbsent {
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
	if patch.Type != config.MergePatch && patch.Type != config.JSONPatch {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("unsupported configuration patch type %q", patch.Type))
	}
	request := c.client.Patch(c.itemPath(namespace, name)).Body(bytes.NewReader(patch.Data), string(patch.Type))
	setWritePreconditionHeaders(request, false, write.ExpectedVersion)
	result := &config.Configuration{}
	if _, err := request.Return(result).Do(ctx); err != nil {
		return nil, err
	}
	if object != nil {
		if err := unmarshalObject(result.Value, object); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *DynamicConfig) Watch(ctx context.Context, namespace, name string) (config.Watcher, error) {
	watchContext, cancel := context.WithCancel(ctx)
	response, err := c.client.Get(c.itemPath(namespace, name)).
		Query("watch", "true").
		Header("Accept", api.ContentTypeEventStream).
		Do(watchContext)
	if err != nil {
		cancel()
		return nil, err
	}
	decoder, err := api.NewStreamDecoderFromResponse[configurationWatchEvent](response)
	if err != nil {
		cancel()
		response.Body.Close()
		return nil, err
	}
	watcher := &configurationWatcher{
		body:    response.Body,
		cancel:  cancel,
		events:  make(chan config.Event),
		stopped: make(chan struct{}),
	}
	go watcher.run(watchContext, decoder)
	return watcher, nil
}

type configurationWatchEvent struct {
	Configuration *config.Configuration `json:"configuration,omitempty"`
}

type configurationWatcher struct {
	body     io.ReadCloser
	cancel   context.CancelFunc
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
		w.cancel()
		w.body.Close()
	})
}

func (w *configurationWatcher) run(ctx context.Context, decoder api.StreamDecoder[configurationWatchEvent]) {
	defer close(w.events)
	defer w.Stop()
	initialized := false
	err := decoder.Decode(ctx, func(ctx context.Context, kind string, data configurationWatchEvent) error {
		eventType := config.EventType(kind)
		if eventType != config.EventInitial && eventType != config.EventChange && eventType != config.EventDelete {
			return fmt.Errorf("unsupported configuration event type %q", kind)
		}
		if !initialized && eventType != config.EventInitial {
			return fmt.Errorf("configuration watcher started with %q instead of %q", eventType, config.EventInitial)
		}
		if initialized && eventType == config.EventInitial {
			return fmt.Errorf("configuration watcher returned more than one %q event", config.EventInitial)
		}
		initialized = true
		if eventType == config.EventDelete {
			data.Configuration = nil
		}
		if !w.send(ctx, config.Event{Type: eventType, Configuration: data.Configuration}) {
			return context.Canceled
		}
		return nil
	})
	if err != nil && err != context.Canceled && ctx.Err() == nil && !w.wasStopped() {
		w.send(ctx, config.Event{Error: err})
		return
	}
	if err == nil && ctx.Err() == nil && !w.wasStopped() {
		w.send(ctx, config.Event{Error: fmt.Errorf("configuration watcher closed")})
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

func (c *DynamicConfig) collectionPath(namespace string) string {
	return fmt.Sprintf("/namespaces/%s/configurations", namespace)
}

func (c *DynamicConfig) itemPath(namespace, name string) string {
	return c.collectionPath(namespace) + "/" + name
}

func setWritePreconditionHeaders(request *httpclient.Builder, ifAbsent bool, expectedVersion *int64) {
	if ifAbsent {
		request.Header("If-None-Match", "*")
	}
	if expectedVersion != nil {
		request.Header("If-Match", fmt.Sprintf(`"%d"`, *expectedVersion))
	}
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
