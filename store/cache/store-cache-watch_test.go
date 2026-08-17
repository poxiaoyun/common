package cache

import (
	"testing"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
	storeinmemory "xiaoshiai.cn/common/store/inmemory"
)

func TestCacheStoreWatchUsesInitialSnapshotWithoutHistory(t *testing.T) {
	schema := store.NewSchema()
	if err := schema.Register(&TestObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	upstream, err := storeinmemory.New(schema)
	if err != nil {
		t.Fatalf("inmemory.New() error = %v", err)
	}
	object := &TestObject{ObjectMeta: store.ObjectMeta{ID: "one", Name: "one"}}
	if err := upstream.Create(t.Context(), object); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	storage := NewCacheStore(upstream)

	watcher, err := storage.Watch(t.Context(), &store.List[TestObject]{}, store.WithSendInitialEvents())
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	create := receiveCacheWatchEvent(t, watcher)
	bookmark := receiveCacheWatchEvent(t, watcher)
	if create.Type != store.WatchEventCreate || create.Object.GetID() != "one" {
		t.Fatalf("initial event = %#v, want Create one", create)
	}
	if bookmark.Type != store.WatchEventBookmark || bookmark.ResourceVersion != 0 {
		t.Fatalf("bookmark = %#v, want zero ResourceVersion", bookmark)
	}
	if _, err := storage.Watch(t.Context(), &store.List[TestObject]{}, store.WithWatchResourceVersion(1)); !errors.IsResourceExpired(err) {
		t.Fatalf("Watch(resourceVersion=1) error = %v, want ResourceExpired", err)
	}
}

func TestCacheStoreWatchExpiresWhenUpstreamContinuityIsLost(t *testing.T) {
	upstream := &initialWatchStore{
		first:   make(chan store.WatchEvent, 1),
		second:  make(chan store.WatchEvent, 1),
		started: make(chan struct{}),
	}
	upstream.first <- store.WatchEvent{Type: store.WatchEventBookmark}
	upstream.second <- store.WatchEvent{Type: store.WatchEventBookmark}
	storage := NewCacheStore(upstream)

	watcher, err := storage.Watch(t.Context(), &store.List[TestObject]{})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	close(upstream.first)
	event := receiveCacheWatchEvent(t, watcher)
	if !errors.IsResourceExpired(event.Error) {
		t.Fatalf("terminal event error = %v, want ResourceExpired", event.Error)
	}
	select {
	case _, ok := <-watcher.Events():
		if ok {
			t.Fatal("Watch event channel remained open after terminal error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch event channel did not close after terminal error")
	}
}

func TestCacheStoreWatchReportsSelectorMembershipTransitions(t *testing.T) {
	schema := store.NewSchema()
	if err := schema.Register(&TestObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	upstream, err := storeinmemory.New(schema)
	if err != nil {
		t.Fatalf("inmemory.New() error = %v", err)
	}
	object := &TestObject{ObjectMeta: store.ObjectMeta{ID: "one", Labels: map[string]string{"state": "inactive"}}}
	if err := upstream.Create(t.Context(), object); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	storage := NewCacheStore(upstream)
	labelSelector := func(options *store.WatchOptions) {
		options.LabelRequirements = []store.Requirement{store.RequirementEqual("state", "active")}
	}
	watcher, err := storage.Watch(t.Context(), &store.List[TestObject]{}, store.WithSendInitialEvents(), labelSelector)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	if bookmark := receiveCacheWatchEvent(t, watcher); bookmark.Type != store.WatchEventBookmark {
		t.Fatalf("initial event = %#v, want Bookmark", bookmark)
	}

	object.Labels["state"] = "active"
	if err := upstream.Update(t.Context(), object); err != nil {
		t.Fatalf("Update(active) error = %v", err)
	}
	if event := receiveCacheWatchEvent(t, watcher); event.Type != store.WatchEventCreate {
		t.Fatalf("inactive to active event = %#v, want Create", event)
	}
	object.Name = "updated"
	if err := upstream.Update(t.Context(), object); err != nil {
		t.Fatalf("Update(matching) error = %v", err)
	}
	if event := receiveCacheWatchEvent(t, watcher); event.Type != store.WatchEventUpdate {
		t.Fatalf("active to active event = %#v, want Update", event)
	}
	object.Labels["state"] = "inactive"
	if err := upstream.Update(t.Context(), object); err != nil {
		t.Fatalf("Update(inactive) error = %v", err)
	}
	event := receiveCacheWatchEvent(t, watcher)
	if event.Type != store.WatchEventDelete || event.Object.GetLabels()["state"] != "active" {
		t.Fatalf("active to inactive event = %#v, want Delete with previous object", event)
	}
}

func receiveCacheWatchEvent(t testing.TB, watcher store.Watcher) store.WatchEvent {
	t.Helper()
	select {
	case event, ok := <-watcher.Events():
		if !ok {
			t.Fatal("Watch event channel closed")
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Watch event")
		return store.WatchEvent{}
	}
}
