package etcdcache

import (
	"context"
	"reflect"
	"testing"
	"time"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"xiaoshiai.cn/common/store"
)

func TestEtcdStore_Watch(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
	}

	ctx := context.Background()

	obj := &MyObject{
		ObjectMeta: store.ObjectMeta{ID: "test", Resource: "myobjects"},
		Spec:       MyObjectSpec{Value: "value1"},
	}
	if err := s.Create(ctx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	var watcher store.Watcher
	for range 5 {
		w, err := s.Watch(ctx, &store.List[MyObject]{}, store.WithSendInitialEvents())
		if err != nil {
			t.Logf("failed to create watcher: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		watcher = w
	}
	defer watcher.Stop()

	obj2 := &MyObject{
		ObjectMeta: store.ObjectMeta{ID: "test2", Resource: "myobjects"},
		Spec:       MyObjectSpec{Value: "value2"},
	}
	if err := s.Create(ctx, obj2); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	obj2changed := *obj2
	obj2changed.Spec.Value = "value3"
	if err := s.Update(ctx, &obj2changed); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := receiveEvents(timeoutCtx, 4, watcher)

	expected := []store.WatchEvent{
		{Type: store.WatchEventCreate, Object: obj},
		{Type: store.WatchEventBookmark},
		{Type: store.WatchEventCreate, Object: obj2},
		{Type: store.WatchEventUpdate, Object: &obj2changed},
	}
	for i, event := range events {
		if event.Type != expected[i].Type {
			t.Errorf("expected event type %q, got %q", expected[i].Type, event.Type)
		}
		if event.Type != store.WatchEventBookmark {
			if !reflect.DeepEqual(event.Object, expected[i].Object) {
				t.Errorf("expected event object %+v, got %+v", expected[i].Object, event.Object)
			}
		}
	}
}

func receiveEvents(ctx context.Context, max int, w store.Watcher) []store.WatchEvent {
	events := []store.WatchEvent{}
	for {
		if len(events) >= max {
			return events
		}
		select {
		case <-ctx.Done():
			return events
		case e, ok := <-w.Events():
			if !ok {
				return events
			}
			events = append(events, e)
		}
	}
}
