package api

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

type AuthorizerFunc func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error)

func (f AuthorizerFunc) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	return f(ctx, user, a)
}

type RequestAuthorizerFunc func(r *http.Request) (Decision, string, error)

func (f RequestAuthorizerFunc) AuthorizeRequest(r *http.Request) (Decision, string, error) {
	return f(r)
}

func NewAlwaysAllowAuthorizer() Authorizer {
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		return DecisionAllow, "", nil
	})
}

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

type AuthorizerChain []Authorizer

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

func NewGroupAuthorizer(allow, deny []string) Authorizer {
	return GroupAuthorizer{AllowedGroups: allow, DeniedGroups: deny}
}

type GroupAuthorizer struct {
	AllowedGroups []string
	DeniedGroups  []string
}

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

func NewCacheAuthorizer(authorizer Authorizer, size int, ttl time.Duration) Authorizer {
	return &LRUCacheAuthorizer{
		Authorizer: authorizer,
		cache:      expirable.NewLRU[string, Decision](size, nil, ttl),
	}
}

type LRUCacheAuthorizer struct {
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
