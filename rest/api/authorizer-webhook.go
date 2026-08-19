package api

import (
	"context"
	stderrors "errors"

	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthorizerOptions struct {
	WebhookOptions `json:",inline"`
}

func NewWebhookAuthorizer(opts *WebhookAuthorizerOptions) (*WebhookAuthorizer, error) {
	return NewWebhookAuthorizerWithTransport(opts, nil)
}

// NewWebhookAuthorizerWithTransport creates a Review authorizer whose requests
// use wrapper around the WebhookOptions transport.
func NewWebhookAuthorizerWithTransport(opts *WebhookAuthorizerOptions, wrapper WebhookTransportWrapper) (*WebhookAuthorizer, error) {
	client, err := newHTTPClientFromWebhookOptions(context.Background(), &opts.WebhookOptions, wrapper)
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
