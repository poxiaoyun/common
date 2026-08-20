package api_test

import (
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

type searchListOption string

func (option searchListOption) ApplyToList(options *meta.ListOptions) {
	options.Search = string(option)
}

func TestGetListOptionsAppliesDefaultsToParsedZeroValues(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   meta.ListOptions
	}{
		{
			name:   "omitted",
			target: "/objects",
			want:   meta.ListOptions{Page: 1, Size: 20, Sort: "creationTimestamp-"},
		},
		{
			name:   "explicit page",
			target: "/objects?page=2&size=50&sort=name-",
			want:   meta.ListOptions{Page: 2, Size: 50, Sort: "name-"},
		},
		{
			name:   "empty continuation is omitted",
			target: "/objects?continue=",
			want:   meta.ListOptions{Page: 1, Size: 20, Sort: "creationTimestamp-"},
		},
		{
			name:   "zero numeric pagination uses defaults",
			target: "/objects?page=0&size=0&limit=0",
			want:   meta.ListOptions{Page: 1, Size: 20, Sort: "creationTimestamp-"},
		},
		{
			name:   "page with empty continuation",
			target: "/objects?page=2&size=50&continue=",
			want:   meta.ListOptions{Page: 2, Size: 50, Sort: "creationTimestamp-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tt.target, nil)
			options, err := api.GetListOptions(request, meta.DefaultPage(1, 20), meta.DefaultSort("creationTimestamp-"))
			if err != nil {
				t.Fatal(err)
			}
			if options != tt.want {
				t.Fatalf("options = %#v, want %#v", options, tt.want)
			}
		})
	}
}

func TestGetListOptionsLeavesEmptyPaginationModeNeutral(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?page=&size=&continue=&limit=", nil)
	options, err := api.GetListOptions(request)
	if err != nil {
		t.Fatal(err)
	}
	if options != (meta.ListOptions{}) {
		t.Fatalf("options = %#v, want mode-neutral options", options)
	}
}

func TestGetListOptionsAppliesPaginationDefaultsByField(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		modifier meta.ListOption
		want     meta.ListOptions
	}{
		{name: "continuation limit", target: "/objects", modifier: meta.DefaultContinuation(20), want: meta.ListOptions{Limit: 20}},
		{name: "explicit limit", target: "/objects?limit=50", modifier: meta.DefaultContinuation(20), want: meta.ListOptions{Limit: 50}},
		{name: "page size", target: "/objects?page=2", modifier: meta.DefaultPage(1, 20), want: meta.ListOptions{Page: 2, Size: 20}},
		{name: "page blocks continuation default", target: "/objects?page=2&size=50", modifier: meta.DefaultContinuation(20), want: meta.ListOptions{Page: 2, Size: 50}},
		{name: "page number blocks continuation default", target: "/objects?page=2", modifier: meta.DefaultContinuation(20), want: meta.ListOptions{Page: 2}},
		{name: "continuation blocks page default", target: "/objects?continue=next&limit=50", modifier: meta.DefaultPage(1, 20), want: meta.ListOptions{Continue: "next", Limit: 50}},
		{name: "continuation token blocks page default", target: "/objects?continue=next", modifier: meta.DefaultPage(1, 20), want: meta.ListOptions{Continue: "next"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.target, nil)
			options, err := api.GetListOptions(request, test.modifier)
			if err != nil {
				t.Fatal(err)
			}
			if options != test.want {
				t.Fatalf("options = %#v, want %#v", options, test.want)
			}
		})
	}
}

func TestGetListOptionsPreservesPaginationValuesForServiceSelection(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?page=-1&size=20&continue=next&limit=10", nil)
	options, err := api.GetListOptions(request)
	if err != nil {
		t.Fatal(err)
	}
	want := meta.ListOptions{Page: -1, Size: 20, Continue: "next", Limit: 10}
	if options != want {
		t.Fatalf("options = %#v, want %#v", options, want)
	}
}

func TestGetListOptionsAcceptsExternalConcreteOption(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?search=request", nil)
	options, err := api.GetListOptions(request, searchListOption("worker"))
	if err != nil {
		t.Fatal(err)
	}
	if options.Search != "worker" {
		t.Fatalf("Search = %q, want worker", options.Search)
	}
}
