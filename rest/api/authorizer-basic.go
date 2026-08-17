package api

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// AuthorizerFunc adapts a function to Authorizer.
type AuthorizerFunc func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error)

// Authorize calls f.
func (f AuthorizerFunc) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	return f(ctx, user, a)
}

// RequestAuthorizerFunc adapts a function to RequestAuthorizer.
type RequestAuthorizerFunc func(r *http.Request) (Decision, string, error)

// AuthorizeRequest calls f.
func (f RequestAuthorizerFunc) AuthorizeRequest(r *http.Request) (Decision, string, error) {
	return f(r)
}

// NewAlwaysAllowAuthorizer returns an Authorizer that allows every request.
func NewAlwaysAllowAuthorizer() Authorizer {
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		return DecisionAllow, "", nil
	})
}

// NewAlwaysDenyAuthorizer returns an Authorizer that denies every request.
func NewAlwaysDenyAuthorizer() Authorizer {
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		return DecisionDeny, "", nil
	})
}

// NewWhitelistAuthorizer creates an authorizer that allows access to paths that match any of the given patterns.
// The patterns are regular expressions.
func NewWhitelistAuthorizer(pattern ...string) Authorizer {
	compiledPatterns := make([]*regexp.Regexp, 0, len(pattern))
	for _, pattern := range pattern {
		compiledPatterns = append(compiledPatterns, regexp.MustCompile(pattern))
	}
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		matched := slices.ContainsFunc(compiledPatterns, func(r *regexp.Regexp) bool {
			return r.MatchString(a.Path)
		})
		if matched {
			return DecisionAllow, "", nil
		}
		return DecisionNoOpinion, "", nil
	})
}

// AuthorizerChain evaluates Authorizers in order. Allow, Deny, and errors stop
// evaluation; DecisionNoOpinion continues to the next Authorizer. If every
// Authorizer returns DecisionNoOpinion, the chain denies the request.
type AuthorizerChain []Authorizer

// Authorize evaluates the chain according to AuthorizerChain's ordering and
// short-circuit rules.
func (c AuthorizerChain) Authorize(ctx context.Context, user UserInfo, a Attributes) (Decision, string, error) {
	for _, authorizer := range c {
		decision, reason, err := authorizer.Authorize(ctx, user, a)
		if err != nil {
			return DecisionDeny, reason, err
		}
		if decision == DecisionAllow {
			return DecisionAllow, reason, nil
		}
		if decision == DecisionDeny {
			return DecisionDeny, reason, nil
		}
	}
	return DecisionDeny, "no decision", nil
}

// NewGroupAuthorizer returns an Authorizer that denies members of deny, allows
// members of allow, and otherwise returns DecisionNoOpinion.
func NewGroupAuthorizer(allow, deny []string) Authorizer {
	return GroupAuthorizer{AllowedGroups: allow, DeniedGroups: deny}
}

// GroupAuthorizer authorizes principals by group membership. The first group
// in the principal's Groups that appears in either configured set decides; a
// group present in both sets is denied.
type GroupAuthorizer struct {
	// AllowedGroups contains groups whose members are allowed.
	AllowedGroups []string
	// DeniedGroups contains groups whose members are denied.
	DeniedGroups []string
}

// Authorize checks the principal's groups and returns DecisionNoOpinion when
// none match.
func (g GroupAuthorizer) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	for _, group := range user.Groups {
		if slices.Contains(g.DeniedGroups, group) {
			return DecisionDeny, "", nil
		}
		if slices.Contains(g.AllowedGroups, group) {
			return DecisionAllow, "", nil
		}
	}
	return DecisionNoOpinion, "", nil
}

// NewCacheAuthorizer caches successful decisions from authorizer for ttl.
func NewCacheAuthorizer(authorizer Authorizer, size int, ttl time.Duration) Authorizer {
	return &LRUCacheAuthorizer{
		Authorizer: authorizer,
		cache:      expirable.NewLRU[string, Decision](size, nil, ttl),
	}
}

// LRUCacheAuthorizer caches successful decisions from Authorizer. Denials,
// NoOpinion decisions, and errors are evaluated on every request.
type LRUCacheAuthorizer struct {
	// Authorizer supplies decisions that are not present in the cache.
	Authorizer Authorizer
	cache      *expirable.LRU[string, Decision]
}

// Authorize implements Authorizer.
func (c *LRUCacheAuthorizer) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	if c.cache == nil {
		return c.Authorizer.Authorize(ctx, user, a)
	}
	key := user.Name + "@" + ResourcesToWildcard(a.Resources) + ":" + a.Action
	if decision, ok := c.cache.Get(key); ok {
		return decision, "", nil
	}
	decision, reason, err := c.Authorizer.Authorize(ctx, user, a)
	if err != nil {
		return decision, reason, err
	}
	if decision == DecisionAllow {
		c.cache.Add(key, decision)
	}
	return decision, reason, nil
}
