// Package http implements config.DynamicConfig over IAM's HTTP projection.
package http

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"xiaoshiai.cn/common/config"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
)

var _ config.DynamicConfig = (*DynamicConfig)(nil)

// New returns an HTTP-backed DynamicConfig.
func New(address, token string) (*DynamicConfig, error) {
	client, err := httpclient.NewClientFromOptions(&httpclient.Options{Server: address, Token: token})
	if err != nil {
		return nil, err
	}
	return &DynamicConfig{client: client}, nil
}

// TransportWrapper composes authentication or other request behavior around
// the HTTP adapter's configured base transport.
type TransportWrapper = httpclient.TransportWrapper

// NewWithTransport returns an HTTP-backed DynamicConfig whose requests use
// wrapper around the adapter's configured base transport.
func NewWithTransport(address string, wrapper TransportWrapper) (*DynamicConfig, error) {
	client, err := httpclient.NewClientFromOptionsWithTransport(&httpclient.Options{Server: address}, wrapper)
	if err != nil {
		return nil, err
	}
	return &DynamicConfig{client: client}, nil
}

// DynamicConfig stores configuration through the HTTP projection.
type DynamicConfig struct {
	client *httpclient.Client
}

func (c *DynamicConfig) Set(ctx context.Context, namespace, name string, object any, options ...config.WriteOption) (config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return config.Configuration{}, err
	}
	value, err := config.EncodeObject(object)
	if err != nil {
		return config.Configuration{}, err
	}
	request := c.client.
		Put(c.itemPath(namespace, name)).
		JSON(struct {
			Value config.Object `json:"value"`
		}{Value: value})
	setWritePreconditionHeaders(request, write.ExpectedVersion)
	result := config.Configuration{}
	_, err = request.
		Return(&result).
		Do(ctx)
	if err != nil {
		return config.Configuration{}, err
	}
	return normalizeConfiguration(result)
}

func (c *DynamicConfig) Get(ctx context.Context, namespace, name string, object any) (config.Configuration, error) {
	result := config.Configuration{}
	request := c.client.
		Get(c.itemPath(namespace, name)).
		Return(&result)
	_, err := request.Do(ctx)
	if err != nil {
		if !commonerrors.IsNotFound(err) {
			return config.Configuration{}, err
		}
		result = emptyConfiguration(name)
		if err := result.Value.Decode(object); err != nil {
			return config.Configuration{}, err
		}
		return result, nil
	}
	result, err = normalizeConfiguration(result)
	if err != nil {
		return config.Configuration{}, err
	}
	if err := result.Value.Decode(object); err != nil {
		return config.Configuration{}, err
	}
	return result, nil
}

func (c *DynamicConfig) Patch(ctx context.Context, namespace, name string, patch config.Patch, object any, options ...config.WriteOption) (config.Configuration, error) {
	write, err := config.ResolveWriteOptions(options...)
	if err != nil {
		return config.Configuration{}, err
	}
	if !json.Valid(patch.Data) {
		return config.Configuration{}, commonerrors.NewBadRequest("configuration patch must be valid JSON")
	}
	if patch.Type != config.MergePatch && patch.Type != config.JSONPatch {
		return config.Configuration{}, commonerrors.NewBadRequest(fmt.Sprintf("unsupported configuration patch type %q", patch.Type))
	}
	request := c.client.
		Patch(c.itemPath(namespace, name)).
		Body(bytes.NewReader(patch.Data), string(patch.Type))
	setWritePreconditionHeaders(request, write.ExpectedVersion)
	result := config.Configuration{}
	_, err = request.
		Return(&result).
		Do(ctx)
	if err != nil {
		return config.Configuration{}, err
	}
	result, err = normalizeConfiguration(result)
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

// ListKeys returns the namespace's keys in stable name order.
func (c *DynamicConfig) ListKeys(ctx context.Context, namespace string) ([]config.Key, error) {
	result := []config.Key{}
	request := c.client.
		Get(c.collectionPath(namespace)).
		Return(&result)
	if _, err := request.Do(ctx); err != nil {
		return nil, err
	}
	slices.SortFunc(result, func(left, right config.Key) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return result, nil
}

func (c *DynamicConfig) Watch(ctx context.Context, namespace, name string) (config.Watcher, error) {
	watchContext, cancel := context.WithCancel(ctx)
	response, err := c.client.
		Get(c.itemPath(namespace, name)).
		Query("watch", "true").
		Header("Accept", api.ContentTypeEventStream).
		Do(watchContext)
	if err != nil {
		cancel()
		return nil, err
	}
	decoder, err := api.NewStreamDecoderFromResponse[config.Configuration](response)
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

func (w *configurationWatcher) run(ctx context.Context, decoder api.StreamDecoder[config.Configuration]) {
	defer close(w.events)
	defer w.Stop()
	initialized := false
	err := decoder.Decode(ctx, func(ctx context.Context, _ string, current config.Configuration) error {
		current, err := normalizeConfiguration(current)
		if err != nil {
			return err
		}
		initialized = true
		if !w.send(ctx, config.Event{Configuration: current}) {
			return context.Canceled
		}
		return nil
	})
	if err != nil {
		if err != context.Canceled && ctx.Err() == nil && !w.wasStopped() {
			w.send(ctx, config.Event{Error: err})
		}
		return
	}
	if ctx.Err() != nil || w.wasStopped() {
		return
	}
	message := "configuration watcher closed"
	if !initialized {
		message = "configuration watcher closed before its initial snapshot"
	}
	w.send(ctx, config.Event{Error: errors.New(message)})
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

func setWritePreconditionHeaders(request *httpclient.Builder, expectedVersion *int64) {
	if expectedVersion == nil {
		return
	}
	if *expectedVersion == 0 {
		request.Header("If-None-Match", "*")
		return
	}
	request.Header("If-Match", fmt.Sprintf(`"%d"`, *expectedVersion))
}

func normalizeConfiguration(current config.Configuration) (config.Configuration, error) {
	value, err := config.EncodeObject(current.Value)
	if err != nil {
		return config.Configuration{}, err
	}
	current.Value = value
	return current, nil
}

func emptyConfiguration(name string) config.Configuration {
	return config.Configuration{Name: name, Value: config.Object{}}
}
