package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/etcd"
)

func TestCacheStoreRebuildsFromInitialWatchAfterDisconnect(t *testing.T) {
	upstream := &initialWatchStore{
		first:   make(chan store.WatchEvent, 2),
		second:  make(chan store.WatchEvent, 1),
		started: make(chan struct{}),
	}
	stale := &store.Unstructured{Object: map[string]any{
		"id":              "stale",
		"uid":             "stale-uid",
		"resource":        "testobjects",
		"resourceVersion": int64(1),
	}}
	upstream.first <- store.WatchEvent{Type: store.WatchEventCreate, Object: stale}
	upstream.first <- store.WatchEvent{Type: store.WatchEventBookmark}
	upstream.second <- store.WatchEvent{Type: store.WatchEventBookmark}

	cacheStore := NewCacheStore(upstream)
	list := &store.List[TestObject]{}
	if err := cacheStore.List(t.Context(), list); err != nil {
		t.Fatalf("initial List() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "stale" {
		t.Fatalf("initial List() items = %#v, want stale object", list.Items)
	}

	close(upstream.first)
	select {
	case <-upstream.started:
	case <-time.After(5 * time.Second):
		t.Fatal("cache did not restart initial Watch")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		list = &store.List[TestObject]{}
		if err := cacheStore.List(t.Context(), list); err != nil {
			t.Fatalf("rebuilt List() error = %v", err)
		}
		if len(list.Items) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rebuilt List() items = %#v, want empty authoritative snapshot", list.Items)
		}
		time.Sleep(10 * time.Millisecond)
	}

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	for index, options := range upstream.options {
		if !options.SendInitialEvents || !options.IncludeSubScopes || options.ResourceVersion != nil {
			t.Fatalf("Watch call %d options = %#v, want initial events without resourceVersion", index+1, options)
		}
	}
}

func TestCacheStoreCapabilities(t *testing.T) {
	cacheStore := NewCacheStore(&initialWatchStore{})
	capabilities := cacheStore.Capabilities()
	if !capabilities.Page {
		t.Fatal("Capabilities().Page = false, want true")
	}
	if !capabilities.Watch {
		t.Fatal("Capabilities().Watch = false, want true")
	}
}

func TestCacheStoreRejectsContinuationPagination(t *testing.T) {
	cacheStore := NewCacheStore(&initialWatchStore{})
	err := cacheStore.List(
		t.Context(),
		&store.List[TestObject]{},
		store.WithContinuation("", 10),
	)
	if !errors.IsUnsupported(err) {
		t.Fatalf("List() error = %v, want Unsupported", err)
	}
}

type initialWatchStore struct {
	store.Store
	mu      sync.Mutex
	first   chan store.WatchEvent
	second  chan store.WatchEvent
	started chan struct{}
	options []store.WatchOptions
}

func (*initialWatchStore) Capabilities() store.Capabilities {
	return store.Capabilities{Watch: true}
}

func (s *initialWatchStore) Watch(_ context.Context, _ store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	options := store.ApplyWatchOptions(opts)
	s.mu.Lock()
	s.options = append(s.options, options)
	call := len(s.options)
	s.mu.Unlock()
	if call == 1 {
		return &initialWatcher{events: s.first}, nil
	}
	if call == 2 {
		close(s.started)
	}
	return &initialWatcher{events: s.second}, nil
}

type initialWatcher struct {
	events <-chan store.WatchEvent
}

func (*initialWatcher) Stop() {}

func (w *initialWatcher) Events() <-chan store.WatchEvent { return w.events }

func SetupEtcdTestEtcdStore(t *testing.T) (context.Context, store.Store, func() error) {
	client := testserver.RunEtcd(t, nil)
	schema := store.NewSchema()
	if err := schema.Register(&TestObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	etcdStore, err := etcd.NewEtcdStoreFromClient(client, schema, "/test")
	if err != nil {
		t.Fatalf("create etcd store: %v", err)
	}
	return context.Background(), etcdStore, client.Close
}

type TestObject struct {
	store.ObjectMeta `json:",inline"`
	Spec             TestObjectSpec   `json:"spec,omitempty"`
	Status           TestObjectStatus `json:"status,omitempty"`
}

func (*TestObject) ResourceName() string {
	return "testobjects"
}

type TestObjectSpec struct {
	Replicas *int32 `json:"replicas,omitempty"`
}

type TestObjectStatus struct {
	Phase   string `json:"phase,omitempty"`
	Current int32  `json:"current,omitempty"`
}

func TestCacheStore_Create(t *testing.T) {
	ctx, etcdStore, cleanup := SetupEtcdTestEtcdStore(t)
	defer cleanup()

	testobj1 := &TestObject{ObjectMeta: store.ObjectMeta{ID: "test1", Name: "test1"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}
	testobj2 := &TestObject{ObjectMeta: store.ObjectMeta{ID: "test2", Name: "test2"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}
	testobj3 := &TestObject{ObjectMeta: store.ObjectMeta{ID: "test3", Name: "test3"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}

	objlist := []store.Object{testobj1, testobj2, testobj3}
	for _, obj := range objlist {
		if err := etcdStore.Create(ctx, obj); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
	}

	cacheStore := NewCacheStore(etcdStore)
	testobj1 = &TestObject{}
	if err := cacheStore.Get(ctx, "test1", testobj1); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	list := &store.List[TestObject]{}
	if err := cacheStore.List(ctx, list); err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	if len(list.Items) != len(objlist) {
		t.Fatalf("expected %d, got %d", len(objlist), len(list.Items))
	}
	if err := cacheStore.Scope(store.Scope{Resource: "namespace", Name: "default"}).List(ctx, list); err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected 0, got %d", len(list.Items))
	}

	// update
	testobj1.Spec.Replicas = ptr.To(int32(2))
	if err := cacheStore.Update(ctx, testobj1); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}
	if err := cacheStore.Get(ctx, "test1", testobj1); err != nil {
		if errors.IsNotFound(err) {
			t.Fatalf("failed to get object: %v", err)
		}
		t.Fatalf("failed to get object: %v", err)
	}
}
