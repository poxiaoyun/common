package api

import (
	"context"
	stderrors "errors"

	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthorizerOptions struct {
	// Options configures the AuthorizationReview HTTP endpoint and transport.
	httpclient.Options `json:",inline"`
}

// NewWebhookAuthorizer creates an AuthorizationReview client. ctx owns the
// lifetime of dynamic TLS certificate watchers created for its transport.
func NewWebhookAuthorizer(ctx context.Context, opts *WebhookAuthorizerOptions) (*WebhookAuthorizer, error) {
	return NewWebhookAuthorizerWithTransport(ctx, opts, nil)
}

// NewWebhookAuthorizerWithTransport creates a Review authorizer whose requests
// use wrapper around the configured HTTP transport. ctx owns the lifetime of
// dynamic TLS certificate watchers created for that transport.
func NewWebhookAuthorizerWithTransport(ctx context.Context, opts *WebhookAuthorizerOptions, wrapper httpclient.TransportWrapper) (*WebhookAuthorizer, error) {
	client, err := httpclient.NewClientFromOptionsWithTransport(ctx, &opts.Options, wrapper)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthorizer{httpclient: client}, nil
}

var _ Authorizer = &WebhookAuthorizer{}

type WebhookAuthorizer struct {
	httpclient *httpclient.Client
}

func (t WebhookAuthorizer) Authorize(ctx context.Context, authentication AuthenticationInfo, attr Attributes) (authorized Decision, reason string, err error) {
	review := &AuthorizationReview{Spec: &AuthorizationReviewSpec{
		Authentication: authentication,
		Attributes:     attr,
	}}
	response := &AuthorizationReview{}
	if err := t.httpclient.Post("").JSON(review).Return(response).Send(ctx); err != nil {
		return DecisionNoOpinion, "", err
	}
	if response.Status == nil {
		return DecisionNoOpinion, "", stderrors.New("authorization review returned no status")
	}
	if response.Status.Error != "" {
		return DecisionNoOpinion, response.Status.Reason, stderrors.New(response.Status.Error)
	}
	return response.Status.Decision, response.Status.Reason, nil
}
