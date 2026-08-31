package store_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/selector"
	"xiaoshiai.cn/common/store"
)

func TestListOptionsFromMetaAppliesModifiers(t *testing.T) {
	tenant := selector.RequirementEqual("tenant", "tenant-1")
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
		selector.RequirementEqual("environment", "production"),
		tenant,
	}
	if !reflect.DeepEqual(options.LabelRequirements, wantRequirements) {
		t.Fatalf("LabelRequirements = %#v, want %#v", options.LabelRequirements, wantRequirements)
	}
}

func TestListOptionsFromMetaPreservesContinuationPagination(t *testing.T) {
	modifiers, err := store.ListOptionsFromMeta(meta.ListOptions{Continue: "next", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	options := store.ApplyListOptions(modifiers)
	if options.Continue != "next" || options.Limit != 25 {
		t.Fatalf("pagination = %#v", options)
	}
}

func TestPaginationOptionsApplyThroughPublicSeam(t *testing.T) {
	options := store.ApplyListOptions([]store.ListOption{
		store.WithPage(2, 25),
		store.WithContinuation("next", 10),
	})

	if options.Page != 2 || options.Size != 25 || options.Continue != "next" || options.Limit != 10 {
		t.Fatalf("pagination = %#v", options)
	}
}

func TestListMetadataJSONIncludesOnlySelectedPagination(t *testing.T) {
	tests := []struct {
		name string
		set  func(*store.List[int])
		want string
	}{
		{
			name: "unpaginated",
			set: func(list *store.List[int]) {
				store.SetUnpaginatedListMetadata(list, 0)
			},
			want: `{"items":[],"total":0}`,
		},
		{
			name: "page",
			set: func(list *store.List[int]) {
				store.SetPageListMetadata(list, 2, 20, 0)
			},
			want: `{"items":[],"total":0,"page":2,"size":20}`,
		},
		{
			name: "continuation",
			set: func(list *store.List[int]) {
				store.SetContinuationListMetadata(list, "next", 20)
			},
			want: `{"items":[],"continue":"next","limit":20}`,
		},
		{
			name: "last continuation batch",
			set: func(list *store.List[int]) {
				store.SetContinuationListMetadata(list, "", 20)
			},
			want: `{"items":[],"limit":20}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			list := &store.List[int]{
				Items:    []int{},
				Total:    new(int),
				Page:     9,
				Size:     99,
				Continue: "stale",
				Limit:    99,
			}
			test.set(list)
			encoded, err := json.Marshal(list)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != test.want {
				t.Fatalf("json = %s, want %s", encoded, test.want)
			}
		})
	}
}

func TestListConversionsPreservePaginationMetadata(t *testing.T) {
	total := 7
	list := store.List[int]{
		ResourceVersion: 11,
		Items:           []int{1, 2},
		Total:           &total,
		Page:            2,
		Size:            20,
	}

	converted := store.ConvertList(list, func(value int) string { return string(rune('a' + value - 1)) })
	if converted.ResourceVersion != 11 || converted.Total == nil || *converted.Total != 7 ||
		converted.Page != 2 || converted.Size != 20 {
		t.Fatalf("ConvertList() metadata = %#v", converted)
	}
	if !reflect.DeepEqual(converted.Items, []string{"a", "b"}) {
		t.Fatalf("ConvertList() items = %#v", converted.Items)
	}

	page := store.PageFromList(list)
	if page.ResourceVersion != 11 || page.Total == nil || *page.Total != 7 ||
		page.Page != 2 || page.Size != 20 {
		t.Fatalf("PageFromList() metadata = %#v", page)
	}
	if !reflect.DeepEqual(page.Items, []int{1, 2}) {
		t.Fatalf("PageFromList() items = %#v", page.Items)
	}

	continuation := store.List[int]{Items: []int{3}, Continue: "next", Limit: 25}
	convertedContinuation := store.ConvertList(continuation, func(value int) string { return "c" })
	if convertedContinuation.Total != nil || convertedContinuation.Continue != "next" || convertedContinuation.Limit != 25 {
		t.Fatalf("ConvertList() continuation metadata = %#v", convertedContinuation)
	}
	continuationPage := store.PageFromList(continuation)
	if continuationPage.Total != nil || continuationPage.Continue != "next" || continuationPage.Limit != 25 {
		t.Fatalf("PageFromList() continuation metadata = %#v", continuationPage)
	}
}
