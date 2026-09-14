package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthorizerOptions struct {
	// Options configures the AuthorizationReview HTTP endpoint and transport.
	httpclient.Options `json:",inline"`
}

// NewWebhookAuthorizer creates an AuthorizationReview client.
func NewWebhookAuthorizer(opts *WebhookAuthorizerOptions) (*WebhookAuthorizer, error) {
	return NewWebhookAuthorizerWithTransport(opts, nil)
}

// NewWebhookAuthorizerWithTransport creates a Review authorizer whose requests
// use wrapper around the configured HTTP transport.
func NewWebhookAuthorizerWithTransport(opts *WebhookAuthorizerOptions, wrapper httpclient.TransportWrapper) (*WebhookAuthorizer, error) {
	client, err := httpclient.NewClientFromOptionsWithTransport(&opts.Options, wrapper)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthorizer{httpclient: client}, nil
}

var (
	_ authz.Authorizer                = &WebhookAuthorizer{}
	_ authz.Authorizer                = AuthorizationClient{}
	_ authz.Checker                   = AuthorizationClient{}
	_ authz.BatchChecker              = AuthorizationClient{}
	_ authz.ResourceConstraintPlanner = AuthorizationClient{}
)

// WebhookAuthorizer calls the exact AuthorizationReview endpoint configured in
// WebhookAuthorizerOptions. It is an operation gate, not a final resource check.
type WebhookAuthorizer struct {
	httpclient *httpclient.Client
}

func (t WebhookAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	return authorizeReview(ctx, t.httpclient, "", authentication, operation)
}

// AuthorizationClient implements operation reviews, final resource checks,
// ordered batch checks, and complete collection-constraint plans over HTTP.
// Client's base URL is the authorization API root, for example https://iam/v1;
// its transport authenticates the calling service independently of the supplied
// Authentication being evaluated. Evaluation errors retain structured statuses.
type AuthorizationClient struct {
	Client *httpclient.Client
}

// Authorize evaluates the logical operation gate; NoOpinion remains composable.
func (client AuthorizationClient) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	return authorizeReview(ctx, client.Client, "/authorization-reviews", authentication, operation)
}

func authorizeReview(ctx context.Context, client *httpclient.Client, path string, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	review := &AuthorizationReview{Spec: &AuthorizationReviewSpec{
		Authentication: authentication,
		Operation:      operation,
	}}
	response := &AuthorizationReview{}
	err := client.
		Post(path).
		JSON(review).
		Return(response).
		OnDecode(decodeAuthorizationResponse).
		OnResponse(authorizationStatusOnResponse).
		Send(ctx)
	if err != nil {
		return authz.EvaluationResult{}, err
	}
	if response.Status == nil {
		return authz.EvaluationResult{}, fmt.Errorf("authorization review returned no status")
	}
	switch response.Status.Decision {
	case authz.DecisionAllow, authz.DecisionDeny, authz.DecisionNoOpinion:
		return *response.Status, nil
	default:
		return authz.EvaluationResult{}, fmt.Errorf("authorization review returned invalid decision %q", response.Status.Decision)
	}
}

// Check returns a final Allow or Deny for one concrete resource, or no usable
// result on transport, evaluation, or response-validation failure.
func (client AuthorizationClient) Check(ctx context.Context, authentication Authentication, operation authz.Operation, options ...authz.CheckOption) (authz.EvaluationResult, error) {
	applied := authz.ApplyCheckOptions(options...)
	request := &AuthorizationCheck{Spec: &AuthorizationCheckSpec{
		Authentication: authentication,
		Operation:      operation,
		AtLeast:        applied.AtLeast,
	}}
	response := &AuthorizationCheck{}
	err := client.Client.
		Post("/authorization-checks").
		JSON(request).
		Return(response).
		OnDecode(decodeAuthorizationResponse).
		OnResponse(authorizationStatusOnResponse).
		Send(ctx)
	if err != nil {
		return authz.EvaluationResult{}, err
	}
	if response.Status == nil {
		return authz.EvaluationResult{}, fmt.Errorf("authorization check returned no status")
	}
	if response.Status.Decision != authz.DecisionAllow && response.Status.Decision != authz.DecisionDeny {
		return authz.EvaluationResult{}, fmt.Errorf("authorization check returned non-final decision %q", response.Status.Decision)
	}
	return *response.Status, nil
}

// BatchCheck returns one final decision per operation in input order, or no
// usable result when any part of the batch fails.
func (client AuthorizationClient) BatchCheck(ctx context.Context, authentication Authentication, operations []authz.Operation, options ...authz.BatchCheckOption) (authz.BatchCheckResult, error) {
	applied := authz.ApplyBatchCheckOptions(options...)
	request := &AuthorizationBatchCheck{Spec: &AuthorizationBatchCheckSpec{
		Authentication: authentication,
		Operations:     operations,
		AtLeast:        applied.AtLeast,
	}}
	response := &AuthorizationBatchCheck{}
	err := client.Client.
		Post("/authorization-batch-checks").
		JSON(request).
		Return(response).
		OnDecode(decodeAuthorizationResponse).
		OnResponse(authorizationStatusOnResponse).
		Send(ctx)
	if err != nil {
		return authz.BatchCheckResult{}, err
	}
	if response.Status == nil {
		return authz.BatchCheckResult{}, fmt.Errorf("authorization batch check returned no status")
	}
	if len(response.Status.Decisions) != len(operations) {
		return authz.BatchCheckResult{}, fmt.Errorf("authorization batch check returned %d decisions for %d operations", len(response.Status.Decisions), len(operations))
	}
	for index, result := range response.Status.Decisions {
		if result.Decision != authz.DecisionAllow && result.Decision != authz.DecisionDeny {
			return authz.BatchCheckResult{}, fmt.Errorf("authorization batch check decision %d is non-final: %q", index, result.Decision)
		}
	}
	return *response.Status, nil
}

// PlanResourceConstraint returns a validated complete collection constraint;
// an unavailable or malformed plan never falls back to unrestricted access.
func (client AuthorizationClient) PlanResourceConstraint(ctx context.Context, authentication Authentication, operation authz.Operation, options ...authz.PlanResourceConstraintOption) (authz.ResourceConstraintPlan, error) {
	applied := authz.ApplyPlanResourceConstraintOptions(options...)
	request := &AuthorizationConstraintPlan{Spec: &AuthorizationConstraintPlanSpec{
		Authentication: authentication,
		Operation:      operation,
		AtLeast:        applied.AtLeast,
	}}
	response := &AuthorizationConstraintPlan{}
	err := client.Client.
		Post("/authorization-constraint-plans").
		JSON(request).
		Return(response).
		OnDecode(decodeAuthorizationResponse).
		OnResponse(authorizationStatusOnResponse).
		Send(ctx)
	if err != nil {
		return authz.ResourceConstraintPlan{}, err
	}
	if response.Status == nil {
		return authz.ResourceConstraintPlan{}, fmt.Errorf("authorization constraint plan returned no status")
	}
	if err := response.Status.Constraint.Validate(); err != nil {
		return authz.ResourceConstraintPlan{}, fmt.Errorf("authorization constraint plan returned invalid constraint: %w", err)
	}
	return *response.Status, nil
}

func authorizationStatusOnResponse(request *http.Request, response *http.Response) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return httpclient.StatusOnResponse(request, response)
	}
	return errors.NewCustomError(response.StatusCode, errors.StatusReasonUnknown, "authorization endpoint returned a non-success status")
}

func decodeAuthorizationResponse(_ *http.Request, response *http.Response, into any) error {
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("authorization response contains multiple JSON values")
}
