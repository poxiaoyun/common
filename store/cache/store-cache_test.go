package cache

import (
	"context"
	"testing"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/etcd"
)

func SetupEtcdTestEtcdStore(t *testing.T) (context.Context, store.Store, func() error) {
	client := testserver.RunEtcd(t, nil)
	etcdStore := etcd.NewEtcdStoreFromClient(client, "/test")
	return context.Background(), etcdStore, client.Close
}

type TestObject struct {
	store.ObjectMeta `json:"metadata,omitempty"`
	Spec             TestObjectSpec   `json:"spec,omitempty"`
	Status           TestObjectStatus `json:"status,omitempty"`
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

	testobj1 := &TestObject{ObjectMeta: store.ObjectMeta{Name: "test1", Resource: "test"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}
	testobj2 := &TestObject{ObjectMeta: store.ObjectMeta{Name: "test2", Resource: "test"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}
	testobj3 := &TestObject{ObjectMeta: store.ObjectMeta{Name: "test3", Resource: "test"}, Spec: TestObjectSpec{Replicas: ptr.To(int32(1))}}

	objlist := []store.Object{testobj1, testobj2, testobj3}
	for _, obj := range objlist {
		if err := etcdStore.Create(ctx, obj); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
	}

	cacheStore := NewCacheStore(etcdStore)
	testobj1 = &TestObject{ObjectMeta: store.ObjectMeta{Resource: "test"}}
	if err := cacheStore.Get(ctx, "test1", testobj1); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	list := &store.List[*TestObject]{Resource: "test"}
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
