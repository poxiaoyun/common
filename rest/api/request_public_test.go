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

func TestGetListOptionsAppliesDefaultsOnlyToOmittedValues(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		wantSize int
		wantSort string
	}{
		{name: "omitted", target: "/objects?page=2", wantSize: 20, wantSort: "creationTimestamp-"},
		{name: "explicit", target: "/objects?size=50&sort=name-&continue=next", wantSize: 50, wantSort: "name-"},
		{name: "explicit zero values", target: "/objects?size=0&sort=", wantSize: 0, wantSort: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tt.target, nil)
			options := api.GetListOptions(request, meta.DefaultSize(20), meta.DefaultSort("creationTimestamp-"))
			if options.Size != tt.wantSize || options.Sort != tt.wantSort {
				t.Fatalf("options = %#v", options)
			}
			if tt.name == "omitted" && options.Page != 2 {
				t.Fatalf("Page = %d, want 2", options.Page)
			}
			if tt.name == "explicit" && options.Continue != "next" {
				t.Fatalf("Continue = %q, want next", options.Continue)
			}
		})
	}
}

func TestGetListOptionsAcceptsExternalConcreteOption(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects", nil)
	options := api.GetListOptions(request, searchListOption("worker"))
	if options.Search != "worker" {
		t.Fatalf("Search = %q, want worker", options.Search)
	}
}
