package api

import (
	"context"
	stderrors "errors"

	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthorizerOptions struct {
	Server                string `json:"server,omitempty"`
	ProxyURL              string `json:"proxyURL,omitempty"`
	Token                 string `json:"token,omitempty"`
	Username              string `json:"username,omitempty"`
	Password              string `json:"password,omitempty"`
	CertFile              string `json:"certFile,omitempty"`
	KeyFile               string `json:"keyFile,omitempty"`
	CAFile                string `json:"caFile,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
}

type AuthorizationRequest struct {
	UserInfo   UserInfo   `json:"userInfo,omitempty"`
	Attributes Attributes `json:"attributes,omitempty"`
}

type AuthorizationResponse struct {
	Decision Decision `json:"decision"` // "allow" or "deny"
	Reason   string   `json:"reason,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func NewWebhookAuthorizer(opts *WebhookAuthorizerOptions) (*WebhookAuthorizer, error) {
	config := &httpclient.Config{
		Server:                opts.Server,
		ProxyURL:              opts.ProxyURL,
		Token:                 opts.Token,
		Username:              opts.Username,
		Password:              opts.Password,
		CertFile:              opts.CertFile,
		KeyFile:               opts.KeyFile,
		CAFile:                opts.CAFile,
		InsecureSkipTLSVerify: opts.InsecureSkipTLSVerify,
	}
	cli, err := httpclient.NewClientFromConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthorizer{httpclient: cli}, nil
}

var _ Authorizer = &WebhookAuthorizer{}

type WebhookAuthorizer struct {
	httpclient *httpclient.Client
}

func (t *WebhookAuthorizer) Authorize(ctx context.Context, user UserInfo, attr Attributes) (authorized Decision, reason string, err error) {
	req := &AuthorizationRequest{
		UserInfo:   user,
		Attributes: attr,
	}
	resp := &AuthorizationResponse{}
	if err := t.httpclient.Post("").JSON(req).Return(resp).Send(ctx); err != nil {
		return DecisionNoOpinion, "", err
	}
	if resp.Error != "" {
		return DecisionNoOpinion, resp.Reason, stderrors.New(resp.Error)
	}
	return resp.Decision, resp.Reason, nil
}
