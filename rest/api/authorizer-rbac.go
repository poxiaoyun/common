package api

import (
	"slices"

	"xiaoshiai.cn/common/wildcard"
)

type Authority struct {
	Actions   []string `json:"actions,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

func (a Authority) MatchAttributes(attr Attributes) bool {
	// match action
	if !slices.ContainsFunc(a.Actions, func(act string) bool { return act == "*" || act == attr.Action }) {
		return false
	}
	// match resources
	test := ResourcesToWildcardTest(attr.Resources)
	return slices.ContainsFunc(a.Resources, func(res string) bool {
		return wildcard.Parse(res).Match(test)
	})
}

// return [wildcard.Test] for resources
// e.g. resources: [AttrbuteResource{Resource: "namespaces", Name: "default"}]
// -> "namespaces:default"
func ResourcesToWildcardTest(resources []AttrbuteResource) wildcard.Test {
	tests := make(wildcard.Test, 0, len(resources)*2+1)
	for _, resource := range resources {
		tests = append(tests, resource.Resource, resource.Name)
	}
	return tests
}
