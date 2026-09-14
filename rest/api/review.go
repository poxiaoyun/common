package api

import "xiaoshiai.cn/common/authz"

// AuthenticationReviewSpec contains exactly one credential to authenticate.
// Audiences applies only to Token credentials.
type AuthenticationReviewSpec struct {
	Token        string `json:"token,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	SSHPublicKey string `json:"sshPublicKey,omitempty"`
	// Audiences is used only when the caller delegates Resource Server audience
	// validation to the review service. Leave it empty when the caller, as Apps
	// and Cloud currently do, validates OAuth access-token audiences locally.
	Audiences []string `json:"audiences,omitempty"`
}

// AuthenticationReviewStatus is the result of an authentication review.
type AuthenticationReviewStatus struct {
	Authenticated  bool            `json:"authenticated"`
	Authentication *Authentication `json:"authentication,omitempty"`
	// Audiences is the validated intersection of the requested and token
	// audiences. It is omitted when the request did not delegate validation.
	Audiences []string `json:"audiences,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// AuthenticationReview requests authentication without persisting a resource.
type AuthenticationReview struct {
	Spec   *AuthenticationReviewSpec   `json:"spec,omitempty"`
	Status *AuthenticationReviewStatus `json:"status,omitempty"`
}

// AuthorizationReviewSpec describes the identity and operation to authorize.
type AuthorizationReviewSpec struct {
	Authentication Authentication  `json:"authentication"`
	Operation      authz.Operation `json:"operation"`
}

// AuthorizationReview requests an authorization decision without persisting a resource.
type AuthorizationReview struct {
	Spec   *AuthorizationReviewSpec `json:"spec,omitempty"`
	Status *authz.EvaluationResult  `json:"status,omitempty"`
}

// AuthorizationCheckSpec contains one concrete resource operation and an
// optional provider-owned authorization freshness lower bound.
type AuthorizationCheckSpec struct {
	Authentication Authentication  `json:"authentication"`
	Operation      authz.Operation `json:"operation"`
	AtLeast        string          `json:"atLeast,omitempty"`
}

// AuthorizationCheck requests a final concrete-resource decision.
type AuthorizationCheck struct {
	Spec   *AuthorizationCheckSpec `json:"spec,omitempty"`
	Status *authz.EvaluationResult `json:"status,omitempty"`
}

// AuthorizationBatchCheckSpec contains an ordered set of concrete operations
// for one Authentication at one authorization state.
type AuthorizationBatchCheckSpec struct {
	Authentication Authentication    `json:"authentication"`
	Operations     []authz.Operation `json:"operations"`
	AtLeast        string            `json:"atLeast,omitempty"`
}

// AuthorizationBatchCheck returns one final decision per input in input order.
type AuthorizationBatchCheck struct {
	Spec   *AuthorizationBatchCheckSpec `json:"spec,omitempty"`
	Status *authz.BatchCheckResult      `json:"status,omitempty"`
}

// AuthorizationConstraintPlanSpec identifies a collection and the operation
// whose complete authorized set must be planned; it contains no business query.
type AuthorizationConstraintPlanSpec struct {
	Authentication Authentication  `json:"authentication"`
	Operation      authz.Operation `json:"operation"`
	AtLeast        string          `json:"atLeast,omitempty"`
}

// AuthorizationConstraintPlan requests a complete candidate-resource constraint.
type AuthorizationConstraintPlan struct {
	Spec   *AuthorizationConstraintPlanSpec `json:"spec,omitempty"`
	Status *authz.ResourceConstraintPlan    `json:"status,omitempty"`
}
