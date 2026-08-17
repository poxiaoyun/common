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
