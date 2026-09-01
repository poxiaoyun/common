// Package authz defines operation authorization, concrete resource checks, and
// authorized resource-query constraints.
package authz

import (
	"context"
	"net/netip"

	"xiaoshiai.cn/common/authn"
)

// Permission selects the cross-product of one service, a set of actions, and
// a set of colon-separated resource wildcard patterns.
type Permission struct {
	Service   string   `json:"service"`
	Actions   []string `json:"actions"`
	Resources []string `json:"resources"`
}

// Properties contains explicitly selected subject, resource, or request facts.
// Callers retain ownership; implementations must copy values they retain after
// the operation returns.
type Properties map[string]any

// Resource identifies the concrete resource snapshot being evaluated.
type Resource struct {
	Type       string
	ID         string
	Scope      Scope
	Revision   string
	Properties Properties
}

// AccessSnapshot identifies the authorization state observed by a decision
// point. Its value is provider-owned and opaque to callers.
type AccessSnapshot string

// Decision is the final result of a concrete authorization evaluation.
type Decision string

const (
	// DecisionDeny means the access attempt is not authorized.
	DecisionDeny Decision = "deny"
	// DecisionAllow means the access attempt is authorized.
	DecisionAllow Decision = "allow"
)

// CheckInput is one complete subject, service, action, resource, and trusted
// request-context proposition to check.
type CheckInput struct {
	Subject  authn.Subject
	Service  ServiceID
	Action   string
	Resource Resource
	// RequestIP is the trusted remote request address. Its zero value means the
	// request address is unavailable.
	RequestIP netip.Addr
	Request   Properties
}

// CheckResult is one concrete decision and the authorization snapshot used.
// Reason is diagnostic and must not be parsed for policy semantics.
type CheckResult struct {
	Decision Decision
	Reason   string
	Snapshot AccessSnapshot
}

// CheckOptions contains optional execution controls for one check.
type CheckOptions struct {
	AtLeast AccessSnapshot
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
type AtLeastOption AccessSnapshot

// WithAtLeast requests authorization state no older than snapshot.
func WithAtLeast(snapshot AccessSnapshot) AtLeastOption {
	return AtLeastOption(snapshot)
}

// ApplyToCheck applies the consistency lower bound to one check.
func (option AtLeastOption) ApplyToCheck(options *CheckOptions) {
	options.AtLeast = AccessSnapshot(option)
}

// Checker decides access to one concrete resource snapshot.
type Checker interface {
	// Check decides whether the access proposition is allowed. An error means no
	// decision was produced; the caller must not perform the protected operation.
	Check(ctx context.Context, input CheckInput, options ...CheckOption) (CheckResult, error)
}

// BatchCheckOptions contains optional execution controls for one batch.
type BatchCheckOptions struct {
	AtLeast AccessSnapshot
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
	options.AtLeast = AccessSnapshot(option)
}

// CheckDecision is one item in a batch result. Reason is diagnostic and
// must not be parsed for policy semantics.
type CheckDecision struct {
	Decision Decision
	Reason   string
}

// BatchCheckResult contains decisions in the same order and with the same
// length as the input checks.
type BatchCheckResult struct {
	Decisions []CheckDecision
	Snapshot  AccessSnapshot
}

// BatchChecker decides a finite set of concrete access propositions.
type BatchChecker interface {
	// BatchCheck checks every input at the returned snapshot. It either
	// returns one ordered decision per input or an error and no usable result.
	BatchCheck(ctx context.Context, inputs []CheckInput, options ...BatchCheckOption) (BatchCheckResult, error)
}

// PlanResourceConstraintInput asks for the access constraint on one resource
// type for a subject and action.
type PlanResourceConstraintInput struct {
	Subject      authn.Subject
	Service      ServiceID
	Action       string
	Scope        Scope
	ResourceType string
	// RequestIP is the trusted remote request address. Its zero value means the
	// request address is unavailable.
	RequestIP netip.Addr
	Request   Properties
}

// ResourceConstraintPlan contains the constraint and authorization snapshot
// used to produce it.
type ResourceConstraintPlan struct {
	// Constraint is complete for the plan input.
	Constraint ResourceConstraint
	Snapshot   AccessSnapshot
}

// PlanResourceConstraintOptions contains optional execution controls for one
// resource constraint plan.
type PlanResourceConstraintOptions struct {
	AtLeast AccessSnapshot
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
	options.AtLeast = AccessSnapshot(option)
}

// ResourceConstraintPlanner plans authorization as a candidate-resource
// constraint that a resource domain can combine with its business query.
type ResourceConstraintPlanner interface {
	// PlanResourceConstraint returns a constraint that is complete for input at
	// the returned snapshot. It returns an error when no faithful plan is
	// available; callers must not execute the business query without it.
	PlanResourceConstraint(ctx context.Context, input PlanResourceConstraintInput, options ...PlanResourceConstraintOption) (ResourceConstraintPlan, error)
}
