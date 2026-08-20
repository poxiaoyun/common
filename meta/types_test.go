package meta_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/meta"
)

func TestObjectMetadataResourceVersionJSON(t *testing.T) {
	encoded, err := json.Marshal(meta.ObjectMetadata{ID: "object-1", ResourceVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	written := map[string]any{}
	if err := json.Unmarshal(encoded, &written); err != nil {
		t.Fatal(err)
	}
	if written["resourceVersion"] != float64(7) {
		t.Fatalf("metadata JSON = %s", encoded)
	}

	decoded := meta.ObjectMetadata{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ResourceVersion != 7 {
		t.Fatalf("resource version = %d", decoded.ResourceVersion)
	}
}

func TestPreconditionsJSON(t *testing.T) {
	encoded, err := json.Marshal(meta.Preconditions{UID: "uid-1", ResourceVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"uid":"uid-1","resourceVersion":7}` {
		t.Fatalf("encoded preconditions = %s", encoded)
	}
}

func TestPaginationDefaultsPreserveSelectedBehavior(t *testing.T) {
	pageOptions := meta.ListOptions{Page: 2, Limit: 30}
	meta.DefaultPage(1, 20).ApplyToList(&pageOptions)
	if pageOptions != (meta.ListOptions{Page: 2, Limit: 30}) {
		t.Fatalf("DefaultPage.ApplyToList() = %#v", pageOptions)
	}
	defaultPage := meta.ListOptions{Page: 2}
	meta.DefaultPage(1, 20).ApplyToList(&defaultPage)
	if defaultPage != (meta.ListOptions{Page: 2, Size: 20}) {
		t.Fatalf("DefaultPage.ApplyToList() = %#v", defaultPage)
	}
	continuationIntent := meta.ListOptions{Continue: "next"}
	meta.DefaultPage(1, 20).ApplyToList(&continuationIntent)
	if continuationIntent != (meta.ListOptions{Continue: "next"}) {
		t.Fatalf("DefaultPage.ApplyToList() = %#v", continuationIntent)
	}

	continuationOptions := meta.ListOptions{Page: 2, Size: 20}
	meta.DefaultContinuation(30).ApplyToList(&continuationOptions)
	if continuationOptions != (meta.ListOptions{Page: 2, Size: 20}) {
		t.Fatalf("DefaultContinuation.ApplyToList() = %#v", continuationOptions)
	}
	pageIntent := meta.ListOptions{Page: 2}
	meta.DefaultContinuation(30).ApplyToList(&pageIntent)
	if pageIntent != (meta.ListOptions{Page: 2}) {
		t.Fatalf("DefaultContinuation.ApplyToList() = %#v", pageIntent)
	}

	firstContinuationBatch := meta.ListOptions{Continue: "next"}
	meta.DefaultContinuation(30).ApplyToList(&firstContinuationBatch)
	if firstContinuationBatch != (meta.ListOptions{Continue: "next", Limit: 30}) {
		t.Fatalf("DefaultContinuation.ApplyToList() = %#v", firstContinuationBatch)
	}

	pageDefaultFirst := meta.ApplyListOptions([]meta.ListOption{
		meta.DefaultPage(1, 20),
		meta.DefaultContinuation(30),
	})
	if pageDefaultFirst != (meta.ListOptions{Page: 1, Size: 20}) {
		t.Fatalf("ApplyListOptions(page first) = %#v", pageDefaultFirst)
	}
	continuationDefaultFirst := meta.ApplyListOptions([]meta.ListOption{
		meta.DefaultContinuation(30),
		meta.DefaultPage(1, 20),
	})
	if continuationDefaultFirst != (meta.ListOptions{Limit: 30}) {
		t.Fatalf("ApplyListOptions(continuation first) = %#v", continuationDefaultFirst)
	}
}

func TestPageJSONIncludesOnlySelectedPagination(t *testing.T) {
	tests := []struct {
		name string
		page meta.Page[int]
		want string
	}{
		{
			name: "unpaginated",
			page: meta.Page[int]{Total: meta.Ptr(0), Items: []int{}},
			want: `{"total":0,"items":[]}`,
		},
		{
			name: "page",
			page: meta.Page[int]{Total: meta.Ptr(0), Items: []int{}, Page: 2, Size: 20},
			want: `{"total":0,"items":[],"page":2,"size":20}`,
		},
		{
			name: "continuation",
			page: meta.Page[int]{Items: []int{}, Continue: "next", Limit: 20},
			want: `{"items":[],"continue":"next","limit":20}`,
		},
		{
			name: "last continuation batch",
			page: meta.Page[int]{Items: []int{}, Limit: 20},
			want: `{"items":[],"limit":20}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.page)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != test.want {
				t.Fatalf("json = %s, want %s", encoded, test.want)
			}
		})
	}
}

func TestParseSearch(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		search string
		want   []meta.FieldValue
	}{
		{
			name:   "empty search",
			search: "",
			want:   nil,
		},
		{
			name:   "simple search",
			search: "tom",
			want: []meta.FieldValue{
				{Field: "name", Value: "tom"},
			},
		},
		{
			name:   "fielded search",
			search: "name:tom,description:developer",
			want: []meta.FieldValue{
				{Field: "name", Value: "tom"},
				{Field: "description", Value: "developer"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := meta.ParseSearch(tt.search)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSearch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSort(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		sort string
		want []meta.SortField
	}{
		{
			name: "empty sort",
			sort: "",
			want: nil,
		},
		{
			name: "single ascending sort",
			sort: "name",
			want: []meta.SortField{
				{Field: "name"},
			},
		},
		{
			name: "single descending sort",
			sort: "time-",
			want: []meta.SortField{
				{Field: "time", Direction: meta.SortDirectionDesc},
			},
		},
		{
			name: "legacy suffix sorts",
			sort: "name-,time+",
			want: []meta.SortField{
				{Field: "name", Direction: meta.SortDirectionDesc},
				{Field: "time", Direction: meta.SortDirectionAsc},
			},
		},
		{
			name: "documented prefix sorts",
			sort: "-name,+time",
			want: []meta.SortField{
				{Field: "name", Direction: meta.SortDirectionDesc},
				{Field: "time", Direction: meta.SortDirectionAsc},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := meta.ParseSort(tt.sort)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSort() = %v, want %v", got, tt.want)
			}
		})
	}
}
