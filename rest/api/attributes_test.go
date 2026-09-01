package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestServiceAttributesExtractor(t *testing.T) {
	extractor := api.ServiceAttributesExtractor("cloud", api.PrefixedAttributesExtractor("/v1"))
	attributes, err := extractor(httptest.NewRequest(http.MethodGet, "/v1/clusters", nil))
	if err != nil {
		t.Fatal(err)
	}
	if attributes.Service != "cloud" || attributes.Action != "list" {
		t.Fatalf("attributes = %#v", attributes)
	}
}
