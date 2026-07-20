package inmemory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

type user struct {
	store.ObjectMeta `json:",inline"`
	Email            string `json:"email,omitempty"`
	Phone            string `json:"phone,omitempty"`
}

func (*user) ResourceName() string { return "users" }

func newUserStore(t *testing.T) *InMemory {
	t.Helper()
	schema := store.NewSchema()
	if err := schema.Register(&user{}, store.ResourceSchema{
		ScopeKeys: []string{"tenant"},
		Indexes: []store.Index{
			{Name: "email", Fields: []string{"email"}, Unique: true},
			{Name: "phone", Fields: []string{"phone"}, Unique: true, Nullable: true},
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	storage, err := New(schema)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return storage
}

func TestUniqueIndexes(t *testing.T) {
	ctx := context.Background()
	storage := newUserStore(t)
	tenantA := storage.Scope(store.Scope{Resource: "tenants", Name: "a"})
	tenantB := storage.Scope(store.Scope{Resource: "tenants", Name: "b"})

	first := &user{ObjectMeta: store.ObjectMeta{ID: "one"}, Email: "one@example.com"}
	if err := tenantA.Create(ctx, first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	duplicate := &user{ObjectMeta: store.ObjectMeta{ID: "two"}, Email: first.Email}
	if err := tenantA.Create(ctx, duplicate); !commonerrors.IsAlreadyExists(err) {
		t.Fatalf("Create(duplicate) error = %v, want AlreadyExists", err)
	}
	if err := tenantB.Create(ctx, duplicate); err != nil {
		t.Fatalf("Create(other scope) error = %v", err)
	}
}

func TestUniqueIndexUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	storage := newUserStore(t)
	tenant := storage.Scope(store.Scope{Resource: "tenants", Name: "a"})
	first := &user{ObjectMeta: store.ObjectMeta{ID: "one"}, Email: "old@example.com"}
	if err := tenant.Create(ctx, first); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	first.Email = "new@example.com"
	if err := tenant.Update(ctx, first); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	second := &user{ObjectMeta: store.ObjectMeta{ID: "two"}, Email: "old@example.com"}
	if err := tenant.Create(ctx, second); err != nil {
		t.Fatalf("Create(reused index) error = %v", err)
	}
	if err := tenant.Delete(ctx, first); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	third := &user{ObjectMeta: store.ObjectMeta{ID: "three"}, Email: "new@example.com"}
	if err := tenant.Create(ctx, third); err != nil {
		t.Fatalf("Create(after delete) error = %v", err)
	}
}

func TestNullableUniqueIndex(t *testing.T) {
	ctx := context.Background()
	storage := newUserStore(t).Scope(store.Scope{Resource: "tenants", Name: "a"})
	for _, id := range []string{"one", "two"} {
		if err := storage.Create(ctx, &user{ObjectMeta: store.ObjectMeta{ID: id}, Email: id + "@example.com"}); err != nil {
			t.Fatalf("Create(%s) error = %v", id, err)
		}
	}
}

func TestConcurrentUniqueIndex(t *testing.T) {
	ctx := context.Background()
	storage := newUserStore(t).Scope(store.Scope{Resource: "tenants", Name: "a"})
	var successes atomic.Int32
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			err := storage.Create(ctx, &user{
				ObjectMeta: store.ObjectMeta{ID: fmt.Sprintf("user-%d", i)},
				Email:      "shared@example.com",
			})
			if err == nil {
				successes.Add(1)
				return
			}
			if !commonerrors.IsAlreadyExists(err) {
				t.Errorf("Create() error = %v, want AlreadyExists", err)
			}
		}(i)
	}
	wait.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful creates = %d, want 1", got)
	}
}
