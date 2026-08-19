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

func TestAuthorityMatchesTargetService(t *testing.T) {
	authority := api.Authority{Service: "cloud", Actions: []string{"list"}, Resources: []string{"clusters"}}
	attributes := api.Attributes{Service: "apps", Action: "list", Resources: []api.AttributeResource{{Resource: "clusters"}}}
	if authority.MatchAttributes(attributes) {
		t.Fatal("cloud authority matched Apps attributes")
	}
	attributes.Service = "cloud"
	if !authority.MatchAttributes(attributes) {
		t.Fatal("cloud authority did not match Cloud attributes")
	}
}
