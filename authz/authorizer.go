package authz

import (
	"context"
	"slices"
	"strings"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/pattern"
)

// ResourceSegment identifies one logical resource in an operation target
// path. Name is empty when the target is the resource collection.
type ResourceSegment struct {
	Resource string
	Name     string
}

// AuthorizeInput is one authenticated logical operation to authorize. Context
// contains trusted adapter facts that are not part of the operation identity.
type AuthorizeInput struct {
	Authentication authn.Authentication
	Service        ServiceID
	Action         string
	Resources      []ResourceSegment
	Context        Properties
}

// AuthorizeDecision is one Authorizer's decision about an operation.
type AuthorizeDecision string

const (
	// AuthorizeNoOpinion means the Authorizer is not applicable and another
	// Authorizer may decide.
	AuthorizeNoOpinion AuthorizeDecision = "NoOpinion"
	// AuthorizeDeny rejects the operation.
	AuthorizeDeny AuthorizeDecision = "Deny"
	// AuthorizeAllow permits the operation.
	AuthorizeAllow AuthorizeDecision = "Allow"
)

// AuthorizeResult contains an operation decision and an optional public
// reason. Reason has no machine-readable policy semantics.
type AuthorizeResult struct {
	Decision AuthorizeDecision
	Reason   string
}

// Authorizer decides whether an authenticated logical operation may proceed.
type Authorizer interface {
	// Authorize returns Allow, Deny, or NoOpinion. An error means the operation
	// must not proceed; adapters decide which error details are public.
	Authorize(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error)
}

// AuthorizerFunc adapts a function to Authorizer.
type AuthorizerFunc func(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error)

// Authorize calls f.
func (f AuthorizerFunc) Authorize(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error) {
	return f(ctx, input)
}

// NewAlwaysAllowAuthorizer returns an Authorizer that allows every operation.
func NewAlwaysAllowAuthorizer() AuthorizerFunc {
	return func(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error) {
		return AuthorizeResult{Decision: AuthorizeAllow}, nil
	}
}

// NewAlwaysDenyAuthorizer returns an Authorizer that denies every operation.
func NewAlwaysDenyAuthorizer() AuthorizerFunc {
	return func(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error) {
		return AuthorizeResult{Decision: AuthorizeDeny}, nil
	}
}

// AuthorizerChain evaluates Authorizers in order. Allow, Deny, and errors stop
// evaluation; NoOpinion continues. An all-NoOpinion chain denies.
type AuthorizerChain []Authorizer

// Authorize evaluates the chain according to its ordering and short-circuit
// rules.
func (chain AuthorizerChain) Authorize(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error) {
	for _, authorizer := range chain {
		result, err := authorizer.Authorize(ctx, input)
		if err != nil {
			return result, err
		}
		if result.Decision == AuthorizeAllow || result.Decision == AuthorizeDeny {
			return result, nil
		}
	}
	return AuthorizeResult{Decision: AuthorizeDeny, Reason: "no decision"}, nil
}

// NewGroupAuthorizer returns an Authorizer that denies members of deny, allows
// members of allow, and otherwise returns NoOpinion.
func NewGroupAuthorizer(allow, deny []string) GroupAuthorizer {
	return GroupAuthorizer{AllowedGroups: allow, DeniedGroups: deny}
}

// GroupAuthorizer authorizes operations by authenticated group membership.
// The first authenticated group in either configured set decides; a group in
// both sets is denied.
type GroupAuthorizer struct {
	AllowedGroups []string
	DeniedGroups  []string
}

// Authorize checks the authenticated Subject's groups.
func (authorizer GroupAuthorizer) Authorize(ctx context.Context, input AuthorizeInput) (AuthorizeResult, error) {
	for _, group := range input.Authentication.Groups {
		if slices.Contains(authorizer.DeniedGroups, group) {
			return AuthorizeResult{Decision: AuthorizeDeny}, nil
		}
		if slices.Contains(authorizer.AllowedGroups, group) {
			return AuthorizeResult{Decision: AuthorizeAllow}, nil
		}
	}
	return AuthorizeResult{Decision: AuthorizeNoOpinion}, nil
}

// MatchPermission reports whether permission selects input's Service, Action,
// and logical resource path.
func MatchPermission(permission Permission, input AuthorizeInput) bool {
	if permission.Service != string(input.Service) && permission.Service != "*" {
		return false
	}
	if !slices.ContainsFunc(permission.Actions, func(action string) bool {
		return action == "*" || action == input.Action
	}) {
		return false
	}
	resource := resourcePathValue(input.Resources)
	return slices.ContainsFunc(permission.Resources, func(expression string) bool {
		compiled, err := pattern.CompileWildcard(expression, pattern.WildcardOptions{Separator: ':'})
		return err == nil && compiled.Match(resource)
	})
}

func resourcePathValue(resources []ResourceSegment) string {
	var builder strings.Builder
	for index, resource := range resources {
		if index > 0 {
			builder.WriteString(":")
		}
		builder.WriteString(resource.Resource)
		if index == len(resources)-1 && resource.Name == "" {
			continue
		}
		builder.WriteString(":")
		builder.WriteString(resource.Name)
	}
	return builder.String()
}
