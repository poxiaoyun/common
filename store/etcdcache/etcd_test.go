package etcdcache

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/kubernetes"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/etcd3"
	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/value/encrypt/identity"
	"k8s.io/utils/clock"
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

// TestListPreExistingData reproduces the bug where GetList returns incomplete data
// after upgrading to k8s apiserver v0.35.
//
// The scenario:
// - Create multiple objects through the cacher (which writes to etcd)
// - Wait for cache to be ready and synced
// - List with default options (ResourceVersion=nil → ""), which triggers consistent read from cache
// - Verify all objects are returned
//
// In v0.35, ConsistentListFromCache is GA and locked to true, which means
// List requests with RV="" go through the cache's consistent read path
// instead of falling back to etcd. If the cache is not fully populated,
// incomplete data is returned.
func TestListPreExistingData(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
	}
	ctx := context.Background()

	// Create multiple objects across different scopes
	const objectCount = 10
	for i := range objectCount {
		obj := &MyObject{
			ObjectMeta: store.ObjectMeta{
				ID:   fmt.Sprintf("obj-%d", i),
				Name: fmt.Sprintf("object-%d", i),
			},
			Spec: MyObjectSpec{Value: fmt.Sprintf("value-%d", i)},
		}
		if err := s.Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create object %d: %v", i, err)
		}
	}

	// Also create objects under a scope
	devScope := s.Scope(store.Scope{Resource: "organizations", Name: "dev"})
	const scopedObjectCount = 5
	for i := range scopedObjectCount {
		obj := &MyObject{
			ObjectMeta: store.ObjectMeta{
				ID:   fmt.Sprintf("dev-obj-%d", i),
				Name: fmt.Sprintf("dev-object-%d", i),
			},
			Spec: MyObjectSpec{Value: fmt.Sprintf("dev-value-%d", i)},
		}
		if err := devScope.Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create scoped object %d: %v", i, err)
		}
	}

	// List from root scope with default options (RV=nil → consistent read from cache)
	// This is the path that changed in v0.35: RV="" now goes through cache consistent read
	rootList := &store.List[MyObject]{}
	if err := s.List(ctx, rootList); err != nil {
		t.Fatalf("Failed to list root objects: %v", err)
	}
	t.Logf("Root list returned %d items (expected %d)", len(rootList.Items), objectCount)
	if len(rootList.Items) != objectCount {
		t.Errorf("Root scope: expected %d items, got %d", objectCount, len(rootList.Items))
		for i, item := range rootList.Items {
			t.Logf("  item[%d]: id=%s name=%s", i, item.ID, item.Name)
		}
	}

	// List from dev scope
	devList := &store.List[MyObject]{}
	if err := devScope.List(ctx, devList); err != nil {
		t.Fatalf("Failed to list dev objects: %v", err)
	}
	t.Logf("Dev list returned %d items (expected %d)", len(devList.Items), scopedObjectCount)
	if len(devList.Items) != scopedObjectCount {
		t.Errorf("Dev scope: expected %d items, got %d", scopedObjectCount, len(devList.Items))
		for i, item := range devList.Items {
			t.Logf("  item[%d]: id=%s name=%s", i, item.ID, item.Name)
		}
	}

	// List with IncludeSubScopes from root scope should include all objects
	allList := &store.List[MyObject]{}
	if err := s.List(ctx, allList, store.WithSubScopes()); err != nil {
		t.Fatalf("Failed to list all objects: %v", err)
	}
	t.Logf("All list returned %d items (expected %d)", len(allList.Items), objectCount+scopedObjectCount)
	if len(allList.Items) != objectCount+scopedObjectCount {
		t.Errorf("All scopes: expected %d items, got %d", objectCount+scopedObjectCount, len(allList.Items))
	}
}

// TestListWithNewCacherOnExistingData tests the scenario where data already exists
// in etcd before the cacher is created. This simulates service restart.
//
// This is the most likely scenario for the bug:
// - Data exists in etcd from before the upgrade/restart
// - New cacher is created, starts its reflector to populate the btree cache
// - List requests arrive before the cache is fully populated
func TestListWithNewCacherOnExistingData(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	// Phase: Create data using a first cacher instance
	s1, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create first etcd cacher: %v", err)
	}
	ctx := context.Background()

	const objectCount = 20
	for i := range objectCount {
		obj := &MyObject{
			ObjectMeta: store.ObjectMeta{
				ID:   fmt.Sprintf("existing-%d", i),
				Name: fmt.Sprintf("existing-object-%d", i),
			},
			Spec: MyObjectSpec{Value: fmt.Sprintf("existing-value-%d", i)},
		}
		if err := s1.Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create object %d: %v", i, err)
		}
	}

	// Verify data exists through the first cacher
	list1 := &store.List[MyObject]{}
	if err := s1.List(ctx, list1); err != nil {
		t.Fatalf("Failed to list through first cacher: %v", err)
	}
	if len(list1.Items) != objectCount {
		t.Fatalf("Pre-check: expected %d items through first cacher, got %d", objectCount, len(list1.Items))
	}
	t.Logf("Pre-check passed: %d items exist in etcd", objectCount)

	// Phase: Create a NEW cacher instance (simulates service restart after upgrade)
	// The new cacher needs to initialize its reflector and populate the btree cache
	s2, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create second etcd cacher: %v", err)
	}

	// Try listing IMMEDIATELY after creating the new cacher
	// With RV="" (consistent read), the cacher should either:
	// - Wait for cache to be ready (old behavior)
	// - Return 429 TooManyRequests (new v0.35 behavior with ResilientWatchCacheInitialization GA)
	// - Return data from cache (if cache is already ready)
	immediateList := &store.List[MyObject]{}
	err = s2.List(ctx, immediateList)
	if err != nil {
		t.Logf("Immediate list returned error (expected if cache not ready): %v", err)
		// This is acceptable behavior in v0.35 - cache not ready returns 429
	} else {
		t.Logf("Immediate list returned %d items (expected %d)", len(immediateList.Items), objectCount)
		if len(immediateList.Items) != objectCount {
			t.Errorf("BUG REPRODUCED: immediate list expected %d items, got %d (cache may not be fully populated)",
				objectCount, len(immediateList.Items))
		}
	}

	// Wait for cache to be ready and retry
	var list2 *store.List[MyObject]
	waitErr := waitForCondition(10*time.Second, 100*time.Millisecond, func() (bool, error) {
		list2 = &store.List[MyObject]{}
		if err := s2.List(ctx, list2); err != nil {
			return false, nil // retry on error (cache might not be ready)
		}
		return true, nil
	})
	if waitErr != nil {
		t.Fatalf("Timed out waiting for cache to be ready: %v", waitErr)
	}

	t.Logf("After cache ready: list returned %d items (expected %d)", len(list2.Items), objectCount)
	if len(list2.Items) != objectCount {
		t.Errorf("BUG: After cache ready, expected %d items, got %d", objectCount, len(list2.Items))
		for i, item := range list2.Items {
			t.Logf("  item[%d]: id=%s name=%s rv=%d", i, item.ID, item.Name, item.ResourceVersion)
		}
	}
}

// TestListConsistencyAcrossResourceVersions tests that List returns consistent data
// whether requesting from cache (RV=0) or doing a consistent read (RV="").
func TestListConsistencyAcrossResourceVersions(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
	}
	ctx := context.Background()

	// Create objects
	const objectCount = 15
	for i := range objectCount {
		obj := &MyObject{
			ObjectMeta: store.ObjectMeta{
				ID:   fmt.Sprintf("item-%d", i),
				Name: fmt.Sprintf("item-%d", i),
			},
			Spec: MyObjectSpec{Value: fmt.Sprintf("value-%d", i)},
		}
		if err := s.Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create object %d: %v", i, err)
		}
	}

	// Test: List with RV="" (consistent read - goes through cache in v0.35)
	// ResourceVersion=nil → formatResourceVersion returns ""
	listConsistent := &store.List[MyObject]{}
	if err := s.List(ctx, listConsistent); err != nil {
		t.Fatalf("Failed consistent list (RV=nil/''): %v", err)
	}

	// Test: List with RV=0 (from cache, any version acceptable)
	listCached := &store.List[MyObject]{}
	if err := s.List(ctx, listCached, store.WithResourceVersion(0)); err != nil {
		t.Fatalf("Failed cached list (RV=0): %v", err)
	}

	t.Logf("Consistent read (RV=nil/''): %d items", len(listConsistent.Items))
	t.Logf("Cache read (RV=0):           %d items", len(listCached.Items))

	// Both should return the same count
	if len(listConsistent.Items) != objectCount {
		t.Errorf("BUG: Consistent read returned %d items, expected %d", len(listConsistent.Items), objectCount)
	}
	if len(listCached.Items) != objectCount {
		t.Errorf("BUG: Cache read returned %d items, expected %d", len(listCached.Items), objectCount)
	}

	// They should have the same items
	if len(listConsistent.Items) != len(listCached.Items) {
		t.Errorf("BUG: Inconsistency between consistent read (%d items) and cache read (%d items)",
			len(listConsistent.Items), len(listCached.Items))
	}
}

// TestListScopedAfterRestart tests that scoped lists work correctly after
// creating a new cacher on existing data.
func TestListScopedAfterRestart(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	// Create data using first cacher
	s1, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create first cacher: %v", err)
	}
	ctx := context.Background()

	// Create objects in multiple scopes
	scopes := []store.Scope{
		{Resource: "organizations", Name: "org-a"},
		{Resource: "organizations", Name: "org-b"},
		{Resource: "organizations", Name: "org-c"},
	}

	const itemsPerScope = 5
	for _, scope := range scopes {
		scopedStore := s1.Scope(scope)
		for i := range itemsPerScope {
			obj := &MyObject{
				ObjectMeta: store.ObjectMeta{
					ID:   fmt.Sprintf("%s-item-%d", scope.Name, i),
					Name: fmt.Sprintf("%s-item-%d", scope.Name, i),
				},
				Spec: MyObjectSpec{Value: fmt.Sprintf("%s-value-%d", scope.Name, i)},
			}
			if err := scopedStore.Create(ctx, obj); err != nil {
				t.Fatalf("Failed to create %s/%d: %v", scope.Name, i, err)
			}
		}
	}

	// Create a new cacher (simulates restart)
	s2, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create second cacher: %v", err)
	}

	// Wait for cache to be ready
	waitForCondition(10*time.Second, 100*time.Millisecond, func() (bool, error) {
		list := &store.List[MyObject]{}
		err := s2.Scope(scopes[0]).List(ctx, list)
		return err == nil, nil
	})

	// Verify each scope returns correct count
	for _, scope := range scopes {
		scopedStore := s2.Scope(scope)
		list := &store.List[MyObject]{}
		if err := scopedStore.List(ctx, list); err != nil {
			t.Fatalf("Failed to list scope %s: %v", scope.Name, err)
		}
		t.Logf("Scope %s: got %d items (expected %d)", scope.Name, len(list.Items), itemsPerScope)
		if len(list.Items) != itemsPerScope {
			t.Errorf("BUG: Scope %s expected %d items, got %d", scope.Name, itemsPerScope, len(list.Items))
			for i, item := range list.Items {
				t.Logf("  item[%d]: id=%s scopes=%v", i, item.ID, item.Scopes)
			}
		}
	}
}

// TestDirectCacheVsEtcdComparison directly compares what the CacheDelegator returns
// vs what the underlying etcd3 storage returns. This is the most precise way to
// identify cache incompleteness.
func TestDirectCacheVsEtcdComparison(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create etcd cacher: %v", err)
	}
	ctx := context.Background()

	// Create objects
	const objectCount = 10
	for i := range objectCount {
		obj := &MyObject{
			ObjectMeta: store.ObjectMeta{
				ID:   fmt.Sprintf("direct-%d", i),
				Name: fmt.Sprintf("direct-%d", i),
			},
			Spec: MyObjectSpec{Value: fmt.Sprintf("value-%d", i)},
		}
		if err := s.Create(ctx, obj); err != nil {
			t.Fatalf("Failed to create object %d: %v", i, err)
		}
	}

	// Access internal db to compare cache vs etcd
	resource := s.core.getResource("myobjects")

	// Query via CacheDelegator (cache path) with RV=""
	cacheList := &StorageObjectList{}
	cacheListOpts := storage.ListOptions{
		Recursive: true,
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
		ResourceVersion: "", // consistent read from cache
	}
	keyprefix := "/myobjects/"
	if err := resource.storage.GetList(ctx, keyprefix, cacheListOpts, cacheList); err != nil {
		t.Fatalf("Cache GetList failed: %v", err)
	}
	t.Logf("Cache (RV=''): %d items, resourceVersion=%d", len(cacheList.Items), cacheList.GetResourceVersion())

	// Query via CacheDelegator (cache path) with RV="0"
	cacheList0 := &StorageObjectList{}
	cacheList0Opts := storage.ListOptions{
		Recursive: true,
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
		ResourceVersion: "0", // serve from any cached version
	}
	if err := resource.storage.GetList(ctx, keyprefix, cacheList0Opts, cacheList0); err != nil {
		t.Fatalf("Cache GetList (RV=0) failed: %v", err)
	}
	t.Logf("Cache (RV='0'): %d items, resourceVersion=%d", len(cacheList0.Items), cacheList0.GetResourceVersion())

	// Compare
	if len(cacheList.Items) != objectCount {
		t.Errorf("Cache (RV=''): expected %d items, got %d", objectCount, len(cacheList.Items))
	}
	if len(cacheList0.Items) != objectCount {
		t.Errorf("Cache (RV='0'): expected %d items, got %d", objectCount, len(cacheList0.Items))
	}
	if len(cacheList.Items) != len(cacheList0.Items) {
		t.Errorf("Mismatch: RV='' returned %d items, RV='0' returned %d items",
			len(cacheList.Items), len(cacheList0.Items))
	}
}

// TestCacheAfterDirectEtcdWrite writes data directly to etcd3 storage (bypassing
// the cache), then creates a new cacher and checks if the cache properly picks up
// the pre-existing data. This most closely simulates a restart scenario.
func TestCacheAfterDirectEtcdWrite(t *testing.T) {
	cli := testserver.RunEtcd(t, nil)

	// Create a raw etcd3 storage (not wrapped by cacher)
	groupResource := schema.GroupResource{Resource: "myobjects"}
	transformer := identity.NewEncryptCheckTransformer()
	leaseConfig := etcd3.NewDefaultLeaseManagerConfig()
	newFunc := func() runtime.Object { return &StorageObject{} }
	newListFunc := func() runtime.Object { return &StorageObjectList{} }
	codec := SimpleJsonCodec{}
	versioner := APIObjectVersioner{}
	resourcePrefix := "/" + groupResource.String()
	dec := etcd3.NewDefaultDecoder(codec, versioner)
	compact := etcd3.NewCompactor(cli.Client, time.Hour, clock.RealClock{}, nil)

	rawStorage, err := etcd3.New(cli, compact, codec, newFunc, newListFunc, "/test", resourcePrefix, groupResource, transformer, leaseConfig, dec, versioner)
	if err != nil {
		t.Fatalf("Failed to create raw etcd3 storage: %v", err)
	}

	ctx := context.Background()

	// Write objects directly to etcd (bypassing any cache)
	const objectCount = 15
	for i := range objectCount {
		obj := &StorageObject{
			Object: map[string]any{
				"apiVersion": "v1",
				"resource":   "myobjects",
				"id":         fmt.Sprintf("raw-%d", i),
				"name":       fmt.Sprintf("raw-object-%d", i),
			},
		}
		key := fmt.Sprintf("/myobjects/raw-%d", i)
		out := &StorageObject{}
		if err := rawStorage.Create(ctx, key, obj, out, 0); err != nil {
			t.Fatalf("Failed to create raw object %d: %v", i, err)
		}
	}

	// Verify raw storage has all objects
	rawList := &StorageObjectList{}
	rawListOpts := storage.ListOptions{
		Recursive: true,
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
	}
	if err := rawStorage.GetList(ctx, resourcePrefix, rawListOpts, rawList); err != nil {
		t.Fatalf("Raw storage GetList failed: %v", err)
	}
	t.Logf("Raw etcd3 storage: %d items", len(rawList.Items))
	if len(rawList.Items) != objectCount {
		t.Fatalf("Raw storage expected %d items, got %d", objectCount, len(rawList.Items))
	}

	// Now create a cacher on top of the same etcd data
	s, err := NewEtcdCacherFromClient(cli, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create cacher: %v", err)
	}

	// Wait for cache to be ready
	waitErr := waitForCondition(10*time.Second, 100*time.Millisecond, func() (bool, error) {
		list := &store.List[MyObject]{}
		err := s.List(ctx, list, store.WithSubScopes())
		return err == nil, nil
	})
	if waitErr != nil {
		t.Fatalf("Timed out waiting for cache to be ready: %v", waitErr)
	}

	// List through the cacher with default options (RV="", consistent read)
	cacheList := &store.List[MyObject]{}
	if err := s.List(ctx, cacheList, store.WithSubScopes()); err != nil {
		t.Fatalf("Cache List failed: %v", err)
	}
	t.Logf("Cache list (RV=''): %d items (expected %d)", len(cacheList.Items), objectCount)
	if len(cacheList.Items) != objectCount {
		t.Errorf("BUG: Cache returned %d items, expected %d", len(cacheList.Items), objectCount)
		for i, item := range cacheList.Items {
			t.Logf("  item[%d]: id=%s name=%s", i, item.ID, item.Name)
		}
	}

	// List with RV=0
	cachedList := &store.List[MyObject]{}
	if err := s.List(ctx, cachedList, store.WithSubScopes(), store.WithResourceVersion(0)); err != nil {
		t.Fatalf("Cache List (RV=0) failed: %v", err)
	}
	t.Logf("Cache list (RV=0): %d items (expected %d)", len(cachedList.Items), objectCount)
	if len(cachedList.Items) != objectCount {
		t.Errorf("BUG: Cache (RV=0) returned %d items, expected %d", len(cachedList.Items), objectCount)
	}
}

// TestListFromDevEtcd connects to the real dev etcd and tries to list /iam/roles
// to reproduce the incomplete data bug in a real environment.
//
// Set ETCD_ADDR environment variable to override the default etcd address.
// Default: etcd:2379
//
// Run with: go test -v -run TestListFromDevEtcd -timeout 30s
func TestListFromDevEtcd(t *testing.T) {
	addr := os.Getenv("ETCD_ADDR")
	if addr == "" {
		addr = "etcd:2379"
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{addr},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Skipf("Cannot connect to dev etcd at %s: %v", addr, err)
	}
	defer cli.Close()

	// Quick connectivity check
	checkCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = cli.Get(checkCtx, "/")
	if err != nil {
		t.Skipf("Cannot reach dev etcd at %s: %v", addr, err)
	}
	t.Logf("Connected to dev etcd at %s", addr)

	kubernetescli := kubernetes.Client{Client: cli}
	kubernetescli.Kubernetes = &kubernetescli

	storagePrefix := "/iam"

	s, err := NewEtcdCacherFromClient(&kubernetescli, storagePrefix, nil)
	if err != nil {
		t.Fatalf("Failed to create cacher: %v", err)
	}
	ctx := context.Background()

	// Wait for the cache to be ready
	waitErr := waitForCondition(15*time.Second, 200*time.Millisecond, func() (bool, error) {
		list := &store.List[store.ObjectMeta]{}
		list.SetResource("roles")
		err := s.List(ctx, list, store.WithSubScopes())
		if err != nil {
			t.Logf("Waiting for cache ready: %v", err)
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		t.Fatalf("Cache did not become ready: %v", waitErr)
	}

	// Test: List with RV=\"\" (consistent read from cache) — the path that changed in v0.35
	listConsistent := &store.List[store.ObjectMeta]{}
	listConsistent.SetResource("roles")
	err = s.List(ctx, listConsistent, store.WithSubScopes())
	if err != nil {
		t.Fatalf("Consistent list (RV='') failed: %v", err)
	}
	t.Logf("Consistent read (RV=''): %d items", len(listConsistent.Items))

	// Test: List with RV=0 (from cache, any version)
	listCached := &store.List[store.ObjectMeta]{}
	listCached.SetResource("roles")
	err = s.List(ctx, listCached, store.WithSubScopes(), store.WithResourceVersion(0))
	if err != nil {
		t.Fatalf("Cache list (RV=0) failed: %v", err)
	}
	t.Logf("Cache read (RV=0):       %d items", len(listCached.Items))

	// Also count directly from etcd for comparison
	rawCount := countEtcdKeys(t, cli, storagePrefix+"/roles/")
	t.Logf("Direct etcd count:       %d keys under %s/roles/", rawCount, storagePrefix)

	// Dump raw etcd keys to understand the key structure
	dumpEtcdKeys(t, cli, storagePrefix+"/roles/", 20)

	// Report discrepancies
	if len(listConsistent.Items) != rawCount {
		t.Errorf("BUG: Consistent read returned %d items but etcd has %d keys",
			len(listConsistent.Items), rawCount)
	}
	if len(listCached.Items) != rawCount {
		t.Errorf("BUG: Cache read returned %d items but etcd has %d keys",
			len(listCached.Items), rawCount)
	}
	if len(listConsistent.Items) != len(listCached.Items) {
		t.Errorf("BUG: RV='' returned %d items, RV=0 returned %d items",
			len(listConsistent.Items), len(listCached.Items))
	}

	// Print items for debugging
	for i, item := range listConsistent.Items {
		t.Logf("  [consistent] item[%d]: id=%s name=%s scopes=%v rv=%d",
			i, item.ID, item.Name, item.Scopes, item.ResourceVersion)
	}

	// Also do a raw storage GetList to confirm etcd3 returns all items
	db := s.core.getResource("roles")
	rawList := &StorageObjectList{}
	rawOpts := storage.ListOptions{
		Recursive: true,
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
	}
	if err := db.storage.GetList(ctx, "/roles/", rawOpts, rawList); err != nil {
		t.Logf("Raw GetList through CacheDelegator failed: %v", err)
	} else {
		t.Logf("CacheDelegator GetList:  %d items", len(rawList.Items))
	}
}

func countEtcdKeys(t *testing.T, cli *clientv3.Client, prefix string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithCountOnly())
	if err != nil {
		t.Fatalf("Failed to count etcd keys: %v", err)
	}
	return int(resp.Count)
}

func dumpEtcdKeys(t *testing.T, cli *clientv3.Client, prefix string, limit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithLimit(int64(limit)))
	if err != nil {
		t.Fatalf("Failed to dump etcd keys: %v", err)
	}
	t.Logf("First %d etcd entries under %s:", limit, prefix)
	for i, kv := range resp.Kvs {
		obj := &StorageObject{}
		if err := JsonUnmarshal(kv.Value, &obj.Object); err != nil {
			t.Logf("  key[%d]: %s (mod_rev=%d) DECODE ERROR: %v", i, string(kv.Key), kv.ModRevision, err)
			continue
		}
		scopes, _ := ParseScopes(obj)
		id := GetNestedString(obj.Object, "id")
		name := GetNestedString(obj.Object, "name")
		kind := obj.GetKind()
		generatedKey, keyErr := ScopesObjectKeyFunc(obj)
		t.Logf("  key[%d]: etcdKey=%s id=%s name=%s scopes=%v kind=%s generatedKey=%s keyErr=%v",
			i, string(kv.Key), id, name, scopes, kind, generatedKey, keyErr)
	}
}

func waitForCondition(timeout, interval time.Duration, fn func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := fn()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("condition not met within %v", timeout)
}

// suppress unused import warnings
var (
	_ = schema.GroupResource{}
	_ = identity.NewEncryptCheckTransformer
	_ = etcd3.NewDefaultLeaseManagerConfig
	_ = clock.RealClock{}
	_ storage.Interface = nil
	_ = storagebackend.DefaultEventsHistoryWindow
	_ runtime.Object = nil
	_ = labels.Everything
	_ = fields.Everything
)
