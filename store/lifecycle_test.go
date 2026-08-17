package store_test

import (
	"testing"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/store"
)

func TestPrepareObjectForCreateOwnsServerMetadata(t *testing.T) {
	object := &store.ObjectMeta{UID: "caller", ResourceVersion: 7, Generation: 9}
	scopes := []store.Scope{{Resource: "tenants", Name: "acme"}}
	store.PrepareObjectForCreate(object, "widgets", scopes)
	if _, err := uuid.Parse(object.ID); err != nil {
		t.Fatalf("ID = %q, want UUID: %v", object.ID, err)
	}
	if _, err := uuid.Parse(object.UID); err != nil {
		t.Fatalf("UID = %q, want UUID: %v", object.UID, err)
	}
	if object.ResourceVersion != 0 || object.Generation != 1 || object.CreationTimestamp.IsZero() {
		t.Fatalf("server metadata = %#v", object)
	}
}
