// Package authz defines operation authorization, concrete resource checks, and
// authorized resource-query constraints.
package authz

import (
	"context"

	"xiaoshiai.cn/common/authn"
)

// ResourceReference identifies one concrete resource by type and ID.
type ResourceReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Scope is the authorization instance root when empty, or a concrete
// root-to-resource path otherwise.
type Scope []ResourceReference

// Context contains trusted request-time facts outside the authentication and
// operation identity. Nil means no additional facts. Callers retain ownership;
// implementations must copy values they retain after authorization returns.
type Context map[string]string

// Operation identifies an action on a logical resource or raw path.
// When Resource.Type is empty, Path identifies the raw path target.
type Operation struct {
	// Service identifies the authorization namespace that owns the operation.
	Service string `json:"service"`
	// Action identifies the stable operation being attempted.
	Action string `json:"action"`
	// Resource identifies the resource target. Its Type is empty when Path is
	// the target.
	Resource Resource `json:"resource"`
	// Path identifies a raw path target when Resource.Type is empty.
	Path    string  `json:"path,omitempty"`
	Context Context `json:"context,omitempty"`
}

// Properties contains explicitly selected subject, resource, or request facts.
// Callers retain ownership; implementations must copy values they retain after
// the operation returns.
type Properties map[string]any

// Resource identifies an operation's resource domain or concrete target. An
// Authorizer requires only Type; a Checker also requires ID and may use
// Properties; a ResourceConstraintPlanner requires an empty ID for the
// collection. Scope contains concrete ancestors when the consuming interface
// requires them.
type Resource struct {
	// Type is the globally unambiguous resource-domain type. It is required for
	// every structured resource operation.
	Type string `json:"type,omitempty"`
	// ID identifies one concrete resource. Checker requires it; Authorizer and
	// ResourceConstraintPlanner leave it empty.
	ID string `json:"id,omitempty"`
	// Scope contains the concrete root-to-parent resource path when the resource
	// domain is hierarchical.
	Scope Scope `json:"scope,omitempty"`
	// Properties contains explicitly selected policy facts for Checker. It is
	// optional and is not loaded for Authorizer or ResourceConstraintPlanner.
	Properties Properties `json:"properties,omitempty"`
}

// Decision is the result of an authorization evaluation. NoOpinion is valid
// only for a composable Authorizer; final checks return Allow or Deny.
type Decision string

const (
	// DecisionNoOpinion means the Authorizer is not applicable and another
	// Authorizer may decide.
	DecisionNoOpinion Decision = "NoOpinion"
	// DecisionDeny means the access attempt is not authorized.
	DecisionDeny Decision = "Deny"
	// DecisionAllow means the access attempt is authorized.
	DecisionAllow Decision = "Allow"
)

// EvaluationResult contains an authorization decision and its evaluation
// metadata. Reason is diagnostic and must not be parsed for policy semantics.
// A zero Snapshot means the evaluator did not report an authorization state.
type EvaluationResult struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
	// Snapshot identifies the provider-owned authorization state observed by
	// the evaluation. An empty value means no authorization state was reported.
	Snapshot string `json:"snapshot,omitempty"`
}

// Authorizer decides whether an authenticated logical operation may proceed.
type Authorizer interface {
	// Authorize evaluates an operation gate without requiring Resource.ID or
	// Resource.Properties. It returns Allow, Deny, or NoOpinion; Allow does not
	// replace a final concrete-resource Check. An error means the operation must
	// not proceed; adapters decide which error details are public.
	Authorize(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error)
}

// CheckOptions contains optional execution controls for one check.
type CheckOptions struct {
	AtLeast string
}

// CheckOption configures one check.
type CheckOption interface {
	ApplyToCheck(options *CheckOptions)
}

// ApplyCheckOptions expands options in declaration order.
func ApplyCheckOptions(options ...CheckOption) CheckOptions {
	var applied CheckOptions
	for _, option := range options {
		option.ApplyToCheck(&applied)
	}
	return applied
}

// AtLeastOption requests authorization state no older than one access
// snapshot.
type AtLeastOption string

// WithAtLeast requests authorization state no older than snapshot.
func WithAtLeast(snapshot string) AtLeastOption {
	return AtLeastOption(snapshot)
}

// ApplyToCheck applies the consistency lower bound to one check.
func (option AtLeastOption) ApplyToCheck(options *CheckOptions) {
	options.AtLeast = string(option)
}

// Checker decides access to one concrete resource snapshot.
type Checker interface {
	// Check makes the final decision for an Operation whose Resource has a
	// non-empty Type and ID. Resource.Properties is optional. An error means no
	// decision was produced; the caller must not perform the protected operation.
	Check(ctx context.Context, authentication authn.Authentication, operation Operation, options ...CheckOption) (EvaluationResult, error)
}

// BatchCheckOptions contains optional execution controls for one batch.
type BatchCheckOptions struct {
	AtLeast string
}

// BatchCheckOption configures one batch check.
type BatchCheckOption interface {
	ApplyToBatchCheck(options *BatchCheckOptions)
}

// ApplyBatchCheckOptions expands options in declaration order.
func ApplyBatchCheckOptions(options ...BatchCheckOption) BatchCheckOptions {
	var applied BatchCheckOptions
	for _, option := range options {
		option.ApplyToBatchCheck(&applied)
	}
	return applied
}

// ApplyToBatchCheck applies the consistency lower bound to one batch.
func (option AtLeastOption) ApplyToBatchCheck(options *BatchCheckOptions) {
	options.AtLeast = string(option)
}

// CheckDecision is one item in a batch result. Reason is diagnostic and
// must not be parsed for policy semantics.
type CheckDecision struct {
	Decision Decision
	Reason   string
}

// BatchCheckResult contains decisions in the same order and with the same
// length as the operations.
type BatchCheckResult struct {
	Decisions []CheckDecision
	// Snapshot identifies the provider-owned authorization state shared by all
	// decisions in the batch.
	Snapshot string
}

// BatchChecker decides a finite set of concrete access propositions for one
// authentication.
type BatchChecker interface {
	// BatchCheck checks Operations whose Resources have non-empty Types and IDs
	// at the returned snapshot. It returns one ordered decision per operation or
	// an error and no usable result.
	BatchCheck(ctx context.Context, authentication authn.Authentication, operations []Operation, options ...BatchCheckOption) (BatchCheckResult, error)
}

// ResourceConstraintPlan contains the constraint and authorization snapshot
// used to produce it.
type ResourceConstraintPlan struct {
	// Constraint is complete for the authenticated operation.
	Constraint ResourceConstraint
	// Snapshot identifies the provider-owned authorization state at which the
	// complete Constraint was produced.
	Snapshot string
}

// PlanResourceConstraintOptions contains optional execution controls for one
// resource constraint plan.
type PlanResourceConstraintOptions struct {
	AtLeast string
}

// PlanResourceConstraintOption configures one resource constraint plan.
type PlanResourceConstraintOption interface {
	ApplyToPlanResourceConstraint(options *PlanResourceConstraintOptions)
}

// ApplyPlanResourceConstraintOptions expands options in declaration order.
func ApplyPlanResourceConstraintOptions(options ...PlanResourceConstraintOption) PlanResourceConstraintOptions {
	var applied PlanResourceConstraintOptions
	for _, option := range options {
		option.ApplyToPlanResourceConstraint(&applied)
	}
	return applied
}

// ApplyToPlanResourceConstraint applies the consistency lower bound to one
// resource constraint plan.
func (option AtLeastOption) ApplyToPlanResourceConstraint(options *PlanResourceConstraintOptions) {
	options.AtLeast = string(option)
}

// ResourceConstraintPlanner plans authorization as a candidate-resource
// constraint that a resource domain can combine with its business query.
type ResourceConstraintPlanner interface {
	// PlanResourceConstraint returns a constraint that is complete for the
	// authenticated operation at the returned snapshot. Operation.Resource.ID
	// must be empty because the constraint applies to a resource collection. It
	// returns an error when no faithful plan is available; callers must not
	// execute the business query without it.
	PlanResourceConstraint(ctx context.Context, authentication authn.Authentication, operation Operation, options ...PlanResourceConstraintOption) (ResourceConstraintPlan, error)
}
