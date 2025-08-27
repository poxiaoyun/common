package etcdcache

import (
	"context"
	"testing"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

type MyObject struct {
	store.ObjectMeta `json:",inline"`
	Enabled          bool         `json:"enabled"`
	Spec             MyObjectSpec `json:"spec"`
}

type MyObjectSpec struct {
	Value string `json:"value"`
}

func TestRunResourceCache(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
	}

	obj := &MyObject{
		ObjectMeta: store.ObjectMeta{
			ID:   "test",
			Name: "test",
		},
		Enabled: true,
		Spec:    MyObjectSpec{Value: "some value"},
	}
	ctx := context.Background()
	if err := s.Create(ctx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	devscope := s.Scope(store.Scope{Resource: "organizations", Name: "develop"})

	devobj := &MyObject{
		ObjectMeta: store.ObjectMeta{
			ID:   "test",
			Name: "test",
		},
		Spec: MyObjectSpec{Value: "some dev value"},
	}
	if err := devscope.Create(ctx, devobj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	list := &store.List[MyObject]{}
	if err := s.List(ctx, list); err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(list.Items))
	}

	devobj.Spec.Value = "updated dev value"
	if err := devscope.Update(ctx, devobj); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}
	devobj2 := &MyObject{}
	if err := devscope.Get(ctx, devobj.ID, devobj2); err != nil {
		t.Fatalf("Failed to get object: %v", err)
	}
	if devobj2.Spec.Value != "updated dev value" {
		t.Fatalf("Expected updated value, got %q", devobj2.Spec.Value)
	}
	if err := devscope.Delete(ctx, devobj2); err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}
	geterr := devscope.Get(ctx, devobj.ID, devobj2)
	if geterr == nil {
		t.Fatalf("Expected error getting deleted object, got nil")
	}
	if !errors.IsNotFound(geterr) {
		t.Fatalf("Expected not found error, got: %v", geterr)
	}
}
