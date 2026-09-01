package api

import (
	"context"

	"xiaoshiai.cn/common/authz"
)

// BuildCheckOperationFunc enriches a logical operation with the concrete
// resource ID and policy facts required by Checker. The resource server owns
// this mapping because an operation target does not contain current facts.
type BuildCheckOperationFunc func(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.Operation, error)

// CheckerAuthorizer adapts an authz.Checker to the operation Authorizer seam.
// Its Allow result authorizes only the mapped operation gate; resource domains
// still check concrete current resources before protected reads or effects.
type CheckerAuthorizer struct {
	Checker             authz.Checker
	BuildCheckOperation BuildCheckOperationFunc
}

// NewCheckerAuthorizer returns an operation Authorizer backed by checker.
func NewCheckerAuthorizer(checker authz.Checker, buildCheckOperation BuildCheckOperationFunc) CheckerAuthorizer {
	return CheckerAuthorizer{Checker: checker, BuildCheckOperation: buildCheckOperation}
}

// Authorize maps the operation, checks it, and converts the final decision to
// the operation-gate vocabulary. Unknown decisions deny.
func (authorizer CheckerAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	checkedOperation, err := authorizer.BuildCheckOperation(ctx, authentication, operation)
	if err != nil {
		return authz.EvaluationResult{Decision: authz.DecisionDeny}, err
	}
	result, err := authorizer.Checker.Check(ctx, authentication, checkedOperation)
	if err != nil {
		result.Decision = authz.DecisionDeny
		return result, err
	}
	if result.Decision != authz.DecisionAllow {
		result.Decision = authz.DecisionDeny
	}
	return result, nil
}

var _ authz.Authorizer = CheckerAuthorizer{}
