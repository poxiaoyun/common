package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"xiaoshiai.cn/common/authz"
)

// NewCacheAuthorizer caches successful decisions from authorizer for ttl.
func NewCacheAuthorizer(authorizer authz.Authorizer, size int, ttl time.Duration) *LRUCacheAuthorizer {
	return &LRUCacheAuthorizer{
		Authorizer: authorizer,
		cache:      expirable.NewLRU[[sha256.Size]byte, authz.EvaluationResult](size, nil, ttl),
	}
}

// LRUCacheAuthorizer caches successful decisions from Authorizer. Denials,
// NoOpinion decisions, and errors are evaluated on every request.
type LRUCacheAuthorizer struct {
	// Authorizer supplies decisions that are not present in the cache.
	Authorizer authz.Authorizer
	cache      *expirable.LRU[[sha256.Size]byte, authz.EvaluationResult]
}

// Authorize implements Authorizer.
func (c *LRUCacheAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	if c.cache == nil {
		return c.Authorizer.Authorize(ctx, authentication, operation)
	}
	payload, err := json.Marshal(struct {
		Authentication Authentication
		Operation      authz.Operation
	}{Authentication: authentication, Operation: operation})
	if err != nil {
		return authz.EvaluationResult{Decision: authz.DecisionDeny}, err
	}
	key := sha256.Sum256(payload)
	if result, ok := c.cache.Get(key); ok {
		return result, nil
	}
	result, err := c.Authorizer.Authorize(ctx, authentication, operation)
	if err != nil {
		return result, err
	}
	if result.Decision == authz.DecisionAllow {
		c.cache.Add(key, result)
	}
	return result, nil
}

var _ authz.Authorizer = (*LRUCacheAuthorizer)(nil)
