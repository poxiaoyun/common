package rest_test

import (
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/store"
	storerest "xiaoshiai.cn/common/store/rest"
)

func TestListOptionsFromRequestAppliesRequestDefaults(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?page=2&includeSubscopes=true", nil)
	modifiers, err := storerest.ListOptionsFromRequest(
		request,
		meta.DefaultSize(20),
		meta.DefaultSort("creationTimestamp-"),
	)
	if err != nil {
		t.Fatal(err)
	}
	options := store.ApplyListOptions(modifiers)
	if options.Page != 2 || options.Size != 20 || options.Sort != "creationTimestamp-" {
		t.Fatalf("options = %#v", options)
	}
	if options.IncludeSubScopes {
		t.Fatal("IncludeSubScopes was enabled by a public request")
	}
}

func TestListOptionsFromRequestPreservesExplicitValues(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?size=50&sort=name%2B", nil)
	modifiers, err := storerest.ListOptionsFromRequest(
		request,
		meta.DefaultSize(20),
		meta.DefaultSort("creationTimestamp-"),
	)
	if err != nil {
		t.Fatal(err)
	}
	options := store.ApplyListOptions(modifiers)
	if options.Size != 50 || options.Sort != "name+" {
		t.Fatalf("options = %#v", options)
	}
}
