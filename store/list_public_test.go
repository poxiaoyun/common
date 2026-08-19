package store_test

import (
	"reflect"
	"testing"

	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/store"
)

func TestListOptionsFromMetaAppliesModifiers(t *testing.T) {
	tenant := store.RequirementEqual("tenant", "tenant-1")
	modifiers, err := store.ListOptionsFromMeta(
		meta.ListOptions{
			Page:          2,
			Size:          25,
			LabelSelector: "environment=production",
		},
		store.WithLabelRequirements(tenant),
	)
	if err != nil {
		t.Fatal(err)
	}
	options := store.ApplyListOptions(modifiers)
	if options.Page != 2 || options.Size != 25 {
		t.Fatalf("pagination = %#v", options)
	}
	wantRequirements := store.Requirements{
		store.RequirementEqual("environment", "production"),
		tenant,
	}
	if !reflect.DeepEqual(options.LabelRequirements, wantRequirements) {
		t.Fatalf("LabelRequirements = %#v, want %#v", options.LabelRequirements, wantRequirements)
	}
}
