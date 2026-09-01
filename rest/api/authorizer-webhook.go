package api

import (
	"context"
	stderrors "errors"

	"xiaoshiai.cn/common/authz"
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

var _ authz.Authorizer = &WebhookAuthorizer{}

type WebhookAuthorizer struct {
	httpclient *httpclient.Client
}

func (t WebhookAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	review := &AuthorizationReview{Spec: &AuthorizationReviewSpec{
		Authentication: authentication,
		Attributes:     attributesFromOperation(operation),
	}}
	response := &AuthorizationReview{}
	if err := t.httpclient.Post("").JSON(review).Return(response).Send(ctx); err != nil {
		return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, err
	}
	if response.Status == nil {
		return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, stderrors.New("authorization review returned no status")
	}
	if response.Status.Error != "" {
		return authz.EvaluationResult{
			Decision: authz.DecisionNoOpinion,
			Reason:   response.Status.Reason,
		}, stderrors.New(response.Status.Error)
	}
	return authz.EvaluationResult{Decision: response.Status.Decision, Reason: response.Status.Reason}, nil
}
