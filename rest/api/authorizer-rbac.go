package api

import (
	"slices"
	"strings"

	"xiaoshiai.cn/common/wildcard"
)

type Authority struct {
	Actions   []string `json:"actions,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

func (a Authority) MatchAttributes(attr Attributes) bool {
	return a.MatchActionResource(attr.Action, ResourcesToWildcard(attr.Resources))
}

func MatchAttributes(act, res string, att Attributes) bool {
	if att.Action != act && att.Action != "*" {
		return false
	}
	return wildcard.Match(res, ResourcesToWildcard(att.Resources))
}

func (a Authority) MatchActionResource(act, res string) bool {
	// match action
	if !slices.ContainsFunc(a.Actions, func(item string) bool { return item == "*" || item == act }) {
		return false
	}
	// match resources
	return slices.ContainsFunc(a.Resources, func(item string) bool {
		return wildcard.Match(item, res)
	})
}

// return wildcards for action and expression
// e.g. action: get, resources: [AttrbuteResource{Resource: "namespaces", Name: "default"}]
// -> "get" "namespaces:default"
func ResourcesToWildcard(resources []AttrbuteResource) string {
	builder := strings.Builder{}
	for i, resource := range resources {
		if i > 0 {
			builder.WriteString(":")
		}
		builder.WriteString(resource.Resource)
		if i == len(resources)-1 && resource.Name == "" {
			continue
		}
		builder.WriteString(":")
		builder.WriteString(resource.Name)
	}
	return builder.String()
}
