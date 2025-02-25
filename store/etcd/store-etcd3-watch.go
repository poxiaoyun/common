package etcd

import (
	"bytes"
	"context"
	"fmt"

	etcdrpc "go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

const (
	incomingEventChanSize = 100
	outgoingEventChanSize = 100
)

// Watch implements Store.
func (e *EtcdStore) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return nil, err
	}
	options := &store.WatchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if err := e.core.validateObjectList(obj); err != nil {
		return nil, err
	}
	_, newItemFunc, err := store.NewItemFuncFromList(obj)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	w := &etcdWatcher{
		core:              e.core,
		labelSelector:     options.LabelRequirements,
		fieldSelector:     options.FieldRequirements,
		newItemFunc:       newItemFunc,
		resource:          resource,
		key:               e.core.getlistkey(e.scopes, resource),
		includesubscopes:  options.IncludeSubScopes,
		initialRev:        options.ResourceVersion,
		cancel:            cancel,
		resultChan:        make(chan store.WatchEvent, outgoingEventChanSize),
		incomingEventChan: make(chan *etcdEvent, incomingEventChanSize),
	}
	ctx = clientv3.WithRequireLeader(ctx)
	go w.run(ctx)
	return w, nil
}

type etcdWatcher struct {
	core          *etcdStoreCore
	labelSelector store.Requirements
	fieldSelector store.Requirements
	newItemFunc   func() store.Object
	cancel        func()

	resource          string
	key               string
	includesubscopes  bool
	initialRev        int64
	resultChan        chan store.WatchEvent
	incomingEventChan chan *etcdEvent
}

type etcdEvent struct {
	val        []byte
	rev        int64
	isCreate   bool
	isDelete   bool
	isBookmark bool
}

func (w *etcdWatcher) Stop() {
	w.cancel()
}

func (w *etcdWatcher) Events() <-chan store.WatchEvent {
	return w.resultChan
}

func (w *etcdWatcher) sendError(ctx context.Context, err error) {
	if IsCancelError(err) {
		return
	}
	select {
	case w.resultChan <- store.WatchEvent{Error: errors.NewInternalError(err)}:
	case <-ctx.Done():
	}
}

func (w *etcdWatcher) sendEvent(ctx context.Context, e *etcdEvent) {
	if len(w.incomingEventChan) == cap(w.incomingEventChan) {
		log.V(3).Info("Fast watcher, slow processing. Probably caused by slow decoding, user not receiving fast, or other processing logic",
			"groupResource", w.resource)
	}
	select {
	case w.incomingEventChan <- e:
	case <-ctx.Done():
	}
}

func (w *etcdWatcher) run(ctx context.Context) {
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return w.processEvent(egctx)
	})
	eg.Go(func() error {
		return w.listwatch(egctx)
	})
	if err := eg.Wait(); err != nil {
		// egctx is already Done at this point
		// ctx is ok to use here to send error
		// if ctx is canceled, the error is not sent
		w.sendError(ctx, err)
	}
	w.cancel()
}

func (w *etcdWatcher) listwatch(ctx context.Context) error {
	// list
	if err := w.list(ctx); err != nil {
		return err
	}
	// send bookmark once list is done
	w.sendEvent(ctx, &etcdEvent{isBookmark: true})

	// watch
	opts := []clientv3.OpOption{
		clientv3.WithRev(w.initialRev + 1),
		clientv3.WithPrevKV(),
		clientv3.WithPrefix(),
	}
	watchCh := w.core.client.Watch(ctx, w.key, opts...)
	for wres := range watchCh {
		if err := wres.Err(); err != nil {
			return err
		}
		for _, ev := range wres.Events {
			e := &etcdEvent{
				rev:      ev.Kv.ModRevision,
				val:      ev.Kv.Value,
				isCreate: ev.IsCreate(),
				isDelete: ev.Type == clientv3.EventTypeDelete,
			}
			if e.isDelete && ev.PrevKv != nil {
				e.val = ev.PrevKv.Value
			}
			w.sendEvent(ctx, e)
		}
	}
	return nil
}

func (w *etcdWatcher) list(ctx context.Context) error {
	opts := []clientv3.OpOption{
		clientv3.WithLimit(maxLimit),
		clientv3.WithRange(clientv3.GetPrefixRangeEnd(w.key)),
	}
	if w.initialRev != 0 {
		opts = append(opts, clientv3.WithRev(w.initialRev))
	}

	var err error
	var lastKey []byte
	var getResp *clientv3.GetResponse

	continuekey := w.key
	for {
		getResp, err = w.core.client.KV.Get(ctx, continuekey, opts...)
		if err != nil {
			return interpretListError(w.resource, err)
		}
		if len(getResp.Kvs) == 0 && getResp.More {
			return errors.NewInternalError(fmt.Errorf("no results were found, but etcd indicated there were more values remaining"))
		}
		if w.initialRev == 0 {
			w.initialRev = getResp.Header.Revision
			opts = append(opts, clientv3.WithRev(w.initialRev))
		}
		// send items from the response until no more results
		for i, kv := range getResp.Kvs {
			// has subresources
			if !w.includesubscopes {
				if index := bytes.Index(kv.Key[len(w.key):], []byte("/")); index != -1 {
					continue
				}
			}
			lastKey = kv.Key
			e := &etcdEvent{
				val:      kv.Value,
				rev:      kv.ModRevision,
				isCreate: true,
			}
			w.sendEvent(ctx, e)
			// free kv early. Long lists can take O(seconds) to decode.
			getResp.Kvs[i] = nil
		}
		// no more results remain
		if !getResp.More {
			return nil
		}
		continuekey = string(lastKey) + "\x00"
	}
}

func interpretListError(resource string, err error) error {
	switch {
	case err == etcdrpc.ErrCompacted:
		return errors.NewResourceExpired(resource, "version is compacted")
	}
	return errors.NewInternalError(err)
}

func (w *etcdWatcher) processEvent(ctx context.Context) error {
	for {
		select {
		case e := <-w.incomingEventChan:
			res, err := w.parseEvent(e)
			if err != nil {
				return err
			}
			if res == nil {
				continue
			}
			if len(w.resultChan) == outgoingEventChanSize {
				log.V(3).Info("Fast watcher, slow processing. Probably caused by slow dispatching events to watchers",
					"resource", w.resource)
			}
			// If user couldn't receive results fast enough, we also block incoming events from watcher.
			// Because storing events in local will cause more memory usage.
			// The worst case would be closing the fast watcher.
			select {
			case w.resultChan <- *res:
			case <-ctx.Done():
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (w *etcdWatcher) parseEvent(e *etcdEvent) (*store.WatchEvent, error) {
	if e.isBookmark {
		return &store.WatchEvent{Type: store.WatchEventBookmark}, nil
	}
	obj := w.newItemFunc()
	if err := w.core.serializer.Decode(e.val, obj); err != nil {
		return nil, err
	}
	obj.SetResourceVersion(e.rev)

	// filter by label selector
	if !store.MatchLabelReqirements(obj, w.labelSelector) {
		return nil, nil
	}

	eventType := store.WatchEventUpdate
	if e.isDelete {
		eventType = store.WatchEventDelete
	} else if e.isCreate {
		eventType = store.WatchEventCreate
	}
	event := store.WatchEvent{
		Object: obj,
		Type:   eventType,
	}
	return &event, nil
}

type etcdError interface {
	Code() grpccodes.Code
	Error() string
}

type grpcError interface {
	GRPCStatus() *grpcstatus.Status
}

func IsCancelError(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled {
		return true
	}
	if etcdErr, ok := err.(etcdError); ok && etcdErr.Code() == grpccodes.Canceled {
		return true
	}
	if grpcErr, ok := err.(grpcError); ok && grpcErr.GRPCStatus().Code() == grpccodes.Canceled {
		return true
	}
	return false
}
