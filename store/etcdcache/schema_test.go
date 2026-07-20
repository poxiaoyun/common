package etcdcache

import (
	"context"
	"testing"

	"k8s.io/apiserver/pkg/storage/etcd3/testserver"
	"xiaoshiai.cn/common/store"
)

type indexedObject struct {
	store.ObjectMeta `json:",inline"`
	Email            string `json:"email,omitempty"`
}

func (*indexedObject) ResourceName() string { return "indexedobjects" }

func TestSchemaBuildsCacheIndexes(t *testing.T) {
	schema := store.NewSchema()
	if err := schema.Register(&indexedObject{}, store.ResourceSchema{
		Indexes: []store.Index{{Fields: []string{"email"}}},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	storage, err := NewEtcdCacherFromClient(context.Background(), testserver.RunEtcd(t, nil), schema, "/test")
	if err != nil {
		t.Fatalf("NewEtcdCacherFromClient() error = %v", err)
	}
	fields := storage.core.resourceFields["indexedobjects"]
	if len(fields) != 1 || fields[0] != "email" {
		t.Fatalf("resourceFields = %v, want [email]", fields)
	}
}

func TestSchemaRejectsUniqueIndex(t *testing.T) {
	schema := store.NewSchema()
	if err := schema.Register(&indexedObject{}, store.ResourceSchema{
		Indexes: []store.Index{{Fields: []string{"email"}, Unique: true}},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, err := NewEtcdCacherFromClient(context.Background(), testserver.RunEtcd(t, nil), schema, "/test")
	if err == nil {
		t.Fatal("NewEtcdCacherFromClient() error = nil")
	}
}
