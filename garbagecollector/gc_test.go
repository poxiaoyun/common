package garbagecollector_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/garbagecollector"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/etcdcache"
)

func TestNewChildrenGarbageCollector(t *testing.T) {
	ctx := context.Background()

	etcdstorage, err := etcdcache.NewEtcdCacherFromClient(testserver.RunEtcd(t, nil), "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
		return
	}

	resources := []string{
		"zoos",
		"area",
		"employees",
		"cats",
	}

	storage := etcdstorage
	cgc, err := garbagecollector.NewGarbageCollector(storage, garbagecollector.GarbageCollectorOptions{
		MonitorResources: resources,
	})
	if err != nil {
		t.Fatalf("Failed to create children garbage collector: %v", err)
		return
	}
	go func() {
		if err := cgc.Run(ctx); err != nil {
			panic(err)
		}
	}()

	time.Sleep(1 * time.Second)

	initdatas := []store.ObjectMeta{
		{Name: "main", Resource: "zoos"},
		{Name: "jeff", Resource: "employees", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}}},
		{Name: "lisa", Resource: "employees", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}}},

		{Name: "area1", Resource: "area", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}}},

		{Name: "tom", Resource: "cats", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}, {Resource: "area", Name: "area1"}}},
		{Name: "jerry", Resource: "dogs", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}, {Resource: "area", Name: "area1"}}},

		{Name: "area2", Resource: "area", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}}},
		{Name: "simon", Resource: "cats", Scopes: []store.Scope{{Resource: "zoos", Name: "main"}, {Resource: "area", Name: "area2"}}},

		{Name: "second", Resource: "zoos"},
		{Name: "tony", Resource: "employees", Scopes: []store.Scope{{Resource: "zoos", Name: "second"}}},
		{Name: "lisa", Resource: "employees", Scopes: []store.Scope{{Resource: "zoos", Name: "second"}}},
	}
	for _, data := range initdatas {
		obj := objfrom(data)
		// set owner reference
		setParentScopeReferences(ctx, storage, obj)
		if err := storage.Scope(data.Scopes...).Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create %v: %v", data, err)
		}
	}

	// delete main zoo
	todelete := objfrom(store.ObjectMeta{Name: "main", Resource: "zoos"})
	if err := storage.Scope(todelete.GetScopes()...).Delete(ctx, todelete,
		store.WithDeletePropagation(store.DeletePropagationForeground)); err != nil {
		t.Fatalf("Failed to delete main zoo: %v", err)
	}

	// check if cat, dog, bird are deleted
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for children to be deleted")
		case <-time.After(5 * time.Second):
			todelete := objfrom(store.ObjectMeta{Name: "main", Resource: "zoos"})
			if err := storage.Scope(todelete.GetScopes()...).Get(ctx, todelete.GetName(), todelete); err != nil {
				if errors.IsNotFound(err) {
					t.Log("Main zoo is deleted")
				} else {
					t.Fatalf("Failed to get main zoo: %v", err)
				}
			} else {
				t.Log("Main zoo is not deleted")
			}

			children := store.List[store.Unstructured]{
				Resource: "employees",
			}
			if err := storage.Scope(todelete.GetScopes()...).Scope(store.Scope{Resource: todelete.GetResource(), Name: todelete.GetName()}).List(ctx, &children); err != nil {
				t.Fatalf("Failed to list employees: %v", err)
			}
			if len(children.Items) == 0 {
				t.Log("All children are deleted")
				return
			}
			t.Logf("Waiting for children to be deleted: %v", children.Items)
		}
	}
}

func objfrom(meta store.ObjectMeta) *store.Unstructured {
	uns := store.Unstructured{}
	uns.SetResource(meta.Resource)
	uns.SetName(meta.Name)
	return &uns
}

func setParentScopeReferences(ctx context.Context, root store.Store, obj *store.Unstructured) {
	scopes := obj.GetScopes()
	if len(scopes) == 0 {
		return
	}
	parentscopes, last := scopes[:len(scopes)-1], scopes[len(scopes)-1]

	parent := &store.Unstructured{}
	parent.SetResource(last.Resource)
	if err := root.Scope(parentscopes...).Get(ctx, parent.GetName(), parent); err != nil {
		panic(err)
	}
	obj.SetOwnerReferences([]store.OwnerReference{
		{
			UID:      parent.GetUID(),
			Name:     parent.GetName(),
			Resource: parent.GetResource(),
			Scopes:   parent.GetScopes(),
		},
	})
}
