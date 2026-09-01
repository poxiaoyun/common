package authz

import (
	"slices"
	"strings"

	"xiaoshiai.cn/common/pattern"
)

// Permission selects the cross-product of one service, a set of actions, and
// a set of colon-separated resource wildcard patterns.
type Permission struct {
	Service   string   `json:"service"`
	Actions   []string `json:"actions"`
	Resources []string `json:"resources"`
}

// MatchPermission reports whether permission selects the operation's service,
// action, and resource.
func MatchPermission(permission Permission, operation Operation) bool {
	if permission.Service != string(operation.Service) && permission.Service != "*" {
		return false
	}
	if !slices.ContainsFunc(permission.Actions, func(permissionAction string) bool {
		return permissionAction == "*" || permissionAction == operation.Action
	}) {
		return false
	}
	resource := permissionResourcePath(operation.Resource)
	return slices.ContainsFunc(permission.Resources, func(expression string) bool {
		compiled, err := pattern.CompileWildcard(expression, pattern.WildcardOptions{Separator: ':'})
		return err == nil && compiled.Match(resource)
	})
}

func permissionResourcePath(resource Resource) string {
	var builder strings.Builder
	for index, reference := range resource.Scope {
		if index > 0 {
			builder.WriteString(":")
		}
		builder.WriteString(reference.Type)
		builder.WriteString(":")
		builder.WriteString(reference.ID)
	}
	if resource.Type == "" {
		return builder.String()
	}
	if builder.Len() > 0 {
		builder.WriteString(":")
	}
	builder.WriteString(resource.Type)
	if resource.ID != "" {
		builder.WriteString(":")
		builder.WriteString(resource.ID)
	}
	return builder.String()
}
