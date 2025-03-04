package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

var _ store.Store = &Client{}

func NewRemoteStore(server string) *Client {
	return &Client{cli: httpclient.NewClient(server)}
}

type Client struct {
	cli          *httpclient.Client
	scopesPrefix string
}

// Count implements store.Store.
func (c Client) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return 0, errors.NewBadRequest(err.Error())
	}
	ret := CountResponse{}
	err = c.cli.Get(c.getPath(resource, "")).Query("count", "true").Return(&ret).Send(ctx)
	return ret.Count, err
}

// Create implements store.Store.
func (c Client) Create(ctx context.Context, obj store.Object, opts ...store.CreateOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return c.cli.Post(c.getPath(resource, "")).JSON(obj).Return(obj).Send(ctx)
}

// Delete implements store.Store.
func (c Client) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return c.cli.Delete(c.getPath(resource, obj.GetName())).Return(obj).Send(ctx)
}

// Get implements store.Store.
func (c Client) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return c.cli.Get(c.getPath(resource, name)).Return(obj).Send(ctx)
}

// List implements store.Store.
func (c Client) List(ctx context.Context, list store.ObjectList, opts ...store.ListOption) error {
	resource, err := store.GetResource(list)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return c.cli.Get(c.getPath(resource, "")).Return(list).Send(ctx)
}

// Patch implements store.Store.
func (c Client) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	return c.patch(ctx, obj, false, patch, opts...)
}

// Update implements store.Store.
func (c Client) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	return c.update(ctx, obj, false, opts...)
}

// Watch implements store.Store.
func (c Client) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return nil, errors.NewBadRequest(err.Error())
	}
	options := store.WatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	queries := url.Values{}
	if len(options.LabelRequirements) != 0 {
		queries.Add("labelSelector", options.LabelRequirements.String())
	}
	if len(options.FieldRequirements) != 0 {
		queries.Add("fieldSelector", options.FieldRequirements.String())
	}
	if options.ResourceVersion != 0 {
		queries.Add("resourceVersion", strconv.FormatInt(options.ResourceVersion, 10))
	}
	if options.IncludeSubScopes {
		queries.Add("includeSubscopes", "true")
	}
	if options.SendInitialEvents {
		queries.Add("sendInitialEvents", "true")
	}
	resp, err := c.cli.Get(c.getPath(resource, "")).
		Queries(queries).
		Query("watch", "true").Do(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)

	w := &watcher{
		cancel: cancel,
		result: make(chan store.WatchEvent),
		resp:   resp,
	}
	go w.run(ctx)

	return w, nil
}

type watcher struct {
	cancel context.CancelFunc
	resp   *http.Response
	result chan store.WatchEvent
}

// Events implements store.Watcher.
func (w *watcher) Events() <-chan store.WatchEvent {
	return w.result
}

// Stop implements store.Watcher.
func (w *watcher) Stop() {
	w.cancel()
	close(w.result)
}

func (w *watcher) run(ctx context.Context) {
	defer w.resp.Body.Close()

	err := api.NewSSEDecode(ctx, w.resp.Body, func(e api.Event) error {
		obj := &store.Unstructured{}
		if err := json.Unmarshal(e.Data, obj); err != nil {
			return err
		}
		w.result <- store.WatchEvent{Type: store.WatchEventType(e.Event), Object: obj}
		return nil
	})
	if err != nil {
		w.result <- store.WatchEvent{Error: err}
		return
	} else {
		w.result <- store.WatchEvent{Error: fmt.Errorf("watcher closed")}
		return
	}
}

// Scope implements store.Store.
func (c Client) Scope(scope ...store.Scope) store.Store {
	prefix := c.scopesPrefix
	for _, s := range scope {
		prefix += "/" + s.Resource + "/" + s.Name
	}
	return &Client{cli: c.cli, scopesPrefix: prefix}
}

func (s Client) update(ctx context.Context, obj store.Object, status bool, opts ...store.UpdateOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	options := store.UpdateOptions{}
	for _, o := range opts {
		o(&options)
	}
	queries := url.Values{}
	if options.TTL != 0 {
		queries.Add("ttl", options.TTL.String())
	}
	if len(options.LabelRequirements) != 0 {
		queries.Add("labelSelector", options.LabelRequirements.String())
	}
	if len(options.FieldRequirements) != 0 {
		queries.Add("fieldSelector", options.FieldRequirements.String())
	}
	return s.cli.
		Put(s.getPath(resource, obj.GetName())).
		Query("status", strconv.FormatBool(status)).
		Queries(queries).
		JSON(obj).
		Return(obj).
		Send(ctx)
}

func (c Client) patch(ctx context.Context, obj store.Object, status bool, patch store.Patch, opts ...store.PatchOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	patchdata, err := patch.Data(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	patchtype := patch.Type()

	options := store.PatchOptions{}
	for _, o := range opts {
		o(&options)
	}
	queries := url.Values{}
	if len(options.LabelRequirements) != 0 {
		queries.Add("labelSelector", options.LabelRequirements.String())
	}
	if len(options.FieldRequirements) != 0 {
		queries.Add("fieldSelector", options.FieldRequirements.String())
	}
	return c.cli.Patch(c.getPath(resource, obj.GetName())).
		Body(bytes.NewReader(patchdata), string(patchtype)).
		Query("status", strconv.FormatBool(status)).
		Queries(queries).
		Return(obj).Send(ctx)
}

func (c Client) getPath(resource, name string) string {
	rpath := c.scopesPrefix + "/" + resource
	if name != "" {
		rpath += "/" + name
	}
	return rpath
}

// Status implements store.Store.
func (c Client) Status() store.StatusStorage {
	return &statusClient{Client: c}
}

type statusClient struct {
	Client
}

// Patch implements store.StatusStorage.
func (s *statusClient) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	patchdata, err := patch.Data(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	patchtype := patch.Type()
	return s.cli.
		Patch(s.getPath(resource, obj.GetName())).
		Query("status", "true").
		Body(bytes.NewReader(patchdata), string(patchtype)).
		Return(obj).Send(ctx)
}

// Update implements store.StatusStorage.
func (s *statusClient) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	return s.Client.update(ctx, obj, true, opts...)
}
