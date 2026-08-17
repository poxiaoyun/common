package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// NewCacheAuthorizer caches successful decisions from authorizer for ttl.
func NewCacheAuthorizer(authorizer Authorizer, size int, ttl time.Duration) Authorizer {
	return &LRUCacheAuthorizer{
		Authorizer: authorizer,
		cache:      expirable.NewLRU[[sha256.Size]byte, Decision](size, nil, ttl),
	}
}

// LRUCacheAuthorizer caches successful decisions from Authorizer. Denials,
// NoOpinion decisions, and errors are evaluated on every request.
type LRUCacheAuthorizer struct {
	// Authorizer supplies decisions that are not present in the cache.
	Authorizer Authorizer
	cache      *expirable.LRU[[sha256.Size]byte, Decision]
}

// Authorize implements Authorizer.
func (c *LRUCacheAuthorizer) Authorize(ctx context.Context, authentication AuthenticationInfo, attributes Attributes) (Decision, string, error) {
	if c.cache == nil {
		return c.Authorizer.Authorize(ctx, authentication, attributes)
	}
	payload, err := json.Marshal(struct {
		Authentication AuthenticationInfo `json:"authentication"`
		Attributes     Attributes         `json:"attributes"`
	}{Authentication: authentication, Attributes: attributes})
	if err != nil {
		return DecisionDeny, "", err
	}
	key := sha256.Sum256(payload)
	if decision, ok := c.cache.Get(key); ok {
		return decision, "", nil
	}
	decision, reason, err := c.Authorizer.Authorize(ctx, authentication, attributes)
	if err != nil {
		return decision, reason, err
	}
	if decision == DecisionAllow {
		c.cache.Add(key, decision)
	}
	return decision, reason, nil
}
