package etcd

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

func SetupEtcdTestEtcdStore(t *testing.T) store.Store {
	client := testserver.RunEtcd(t, nil)
	etcdStore := &EtcdStore{core: newEtcdStoreCore(client, "/test")}
	return etcdStore
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

func TestCacheStore_Get(t *testing.T) {
	ctx := context.Background()
	etcdStore := SetupEtcdTestEtcdStore(t)

	obj := &TestObject{
		ObjectMeta: store.ObjectMeta{
			Name:     "test",
			Resource: "test",
		},
		Spec: TestObjectSpec{
			Replicas: ptr.To(int32(1)),
		},
		Status: TestObjectStatus{
			Phase: "Running",
		},
	}
	scopes := []store.Scope{{Resource: "namespace", Name: "default"}}
	namespaceedStore := etcdStore.Scope(scopes...)

	// create
	if err := namespaceedStore.Create(ctx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}
	exists := &TestObject{ObjectMeta: store.ObjectMeta{Resource: "test"}}
	if err := namespaceedStore.Get(ctx, "test", exists); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	if reflect.DeepEqual(obj, exists) {
		t.Fatalf("expected %v, got %v", obj, exists)
	}

	// update
	exists.Spec.Replicas = ptr.To(int32(2))
	if err := namespaceedStore.Update(ctx, exists); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}
	if err := namespaceedStore.Get(ctx, "test", exists); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	if *exists.Spec.Replicas != 2 {
		t.Fatalf("expected 2, got %v", *exists.Spec.Replicas)
	}

	// patch
	patch := store.RawPatch(store.PatchTypeJSONPatch, []byte(`[{"op": "replace", "path": "/spec/replicas", "value": 3}]`))
	if err := namespaceedStore.Patch(ctx, exists, patch); err != nil {
		t.Fatalf("failed to patch object: %v", err)
	}
	if err := namespaceedStore.Get(ctx, "test", exists); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	if *exists.Spec.Replicas != 3 {
		t.Fatalf("expected 3, got %v", *exists.Spec.Replicas)
	}

	// delete
	if err := namespaceedStore.Delete(ctx, exists); err != nil {
		t.Fatalf("failed to delete object: %v", err)
	}
	if err := namespaceedStore.Get(ctx, "test", exists); err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	if exists.DeletionTimestamp == nil {
		t.Fatalf("expected deletion timestamp, got nil")
	}
	if !store.ContainsFinalizer(exists, store.FinalizerDeleteDependents) {
		t.Fatalf("expected finalizer, got none")
	}

	// delete backgroud
	if err := namespaceedStore.Delete(ctx, exists, store.WithDeletePropagation(store.DeletePropagationBackground)); err != nil {
		t.Fatalf("failed to delete object: %v", err)
	}
	if err := namespaceedStore.Get(ctx, "test", exists); err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("expected not found, got %v", err)
		}
	} else {
		t.Fatalf("expected not found, got %v", exists)
	}
}
