package rest

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/store"
)

func TestListOptionsFromRequest(t *testing.T) {
	request := httptest.NewRequest("GET", "/?page=2&size=25&search=worker&sort=name-&continue=next-token&labelSelector=environment%3Dproduction&fieldSelector=enabled%3Dtrue&includeSubscopes=true&resourceVersion=7&fields=id,name", nil)
	options, err := ListOptionsFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if options.Page != 2 || options.Size != 25 || options.Search != "worker" || options.Sort != "name-" || options.Continue != "next-token" {
		t.Fatalf("scalar options = %#v", options)
	}
	if options.ResourceVersion == nil || *options.ResourceVersion != 7 || !options.IncludeSubScopes {
		t.Fatalf("snapshot options = %#v", options)
	}
	if !reflect.DeepEqual(options.Fields, []string{"id", "name"}) {
		t.Fatalf("Fields = %#v", options.Fields)
	}
	if !reflect.DeepEqual(options.LabelRequirements, store.Requirements{store.RequirementEqual("environment", "production")}) {
		t.Fatalf("LabelRequirements = %#v", options.LabelRequirements)
	}
	if !reflect.DeepEqual(options.FieldRequirements, store.Requirements{store.RequirementEqual("enabled", "true")}) {
		t.Fatalf("FieldRequirements = %#v", options.FieldRequirements)
	}
}

func TestListOptionsFromRequestRejectsInvalidResourceVersion(t *testing.T) {
	request := httptest.NewRequest("GET", "/?resourceVersion=current", nil)
	if _, err := ListOptionsFromRequest(request); err == nil {
		t.Fatal("ListOptionsFromRequest returned no error")
	}
}

func TestWatchOptionsFromListOptions(t *testing.T) {
	resourceVersion := int64(7)
	listOptions := store.ListOptions{
		LabelRequirements: store.Requirements{store.RequirementEqual("environment", "production")},
		FieldRequirements: store.Requirements{store.RequirementEqual("enabled", "true")},
		ResourceVersion:   &resourceVersion,
		IncludeSubScopes:  true,
		Page:              2,
		Size:              25,
		Search:            "worker",
		Sort:              "name-",
		Continue:          "next-token",
		Fields:            []string{"id"},
	}
	options := WatchOptionsFromListOptions(listOptions)
	if !reflect.DeepEqual(options.LabelRequirements, listOptions.LabelRequirements) || !reflect.DeepEqual(options.FieldRequirements, listOptions.FieldRequirements) {
		t.Fatalf("selector options = %#v", options)
	}
	if options.ResourceVersion != &resourceVersion || !options.IncludeSubScopes {
		t.Fatalf("snapshot options = %#v", options)
	}
}
