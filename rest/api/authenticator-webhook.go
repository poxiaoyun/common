package api

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthenticatorOptions struct {
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

func NewTokenWebhookAuthenticator(opts *WebhookAuthenticatorOptions) (*TokenWebhookAuthenticator, error) {
	processor, err := NewWebhookAuthenticatorProcessor(opts)
	if err != nil {
		return nil, err
	}
	return &TokenWebhookAuthenticator{Process: processor}, nil
}

var _ TokenAuthenticator = &TokenWebhookAuthenticator{}

type TokenWebhookAuthenticator struct {
	Process *WebhookAuthenticatorProcessor
}

// Authenticate implements TokenAuthenticator.
func (t *TokenWebhookAuthenticator) Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error) {
	return t.Process.Process(ctx, &WebhookAuthenticationRequest{Token: token})
}

var _ BasicAuthenticator = &BasicAuthWebhookAuthenticator{}

func NewBasicAuthWebhookAuthenticator(opts *WebhookAuthenticatorOptions) (*BasicAuthWebhookAuthenticator, error) {
	processor, err := NewWebhookAuthenticatorProcessor(opts)
	if err != nil {
		return nil, err
	}
	return &BasicAuthWebhookAuthenticator{Process: processor}, nil
}

type BasicAuthWebhookAuthenticator struct {
	Process *WebhookAuthenticatorProcessor
}

// Authenticate implements TokenAuthenticator.
func (t *BasicAuthWebhookAuthenticator) Authenticate(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return t.Process.Process(ctx, &WebhookAuthenticationRequest{Username: username, Password: password})
}

func NewWebhookAuthenticator(opts *WebhookAuthenticatorOptions, getRequest func(r *http.Request) (*WebhookAuthenticationRequest, error)) (*WebhookAuthenticator, error) {
	processor, err := NewWebhookAuthenticatorProcessor(opts)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthenticator{GetRequest: getRequest, Process: processor}, nil
}

type WebhookAuthenticator struct {
	GetRequest func(r *http.Request) (*WebhookAuthenticationRequest, error)
	Process    *WebhookAuthenticatorProcessor
}

var _ Authenticator = &WebhookAuthenticator{}

func (w *WebhookAuthenticator) Authenticate(wr http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	req, err := w.GetRequest(r)
	if err != nil {
		return nil, err
	}
	return w.Process.Process(r.Context(), req)
}

func NewWebhookAuthenticatorProcessor(opts *WebhookAuthenticatorOptions) (*WebhookAuthenticatorProcessor, error) {
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
	return &WebhookAuthenticatorProcessor{httpclient: cli}, nil
}

type WebhookAuthenticatorProcessor struct {
	httpclient *httpclient.Client
}

type WebhookAuthenticationRequest struct {
	Token     string   `json:"token"`
	Username  string   `json:"username,omitempty"`
	Password  string   `json:"password,omitempty"`
	Audiences []string `json:"audiences,omitempty"`
}

type WebhookAuthenticationResponse struct {
	Authenticated bool     `json:"authenticated"`
	UserInfo      UserInfo `json:"userInfo,omitempty"`
	Audiences     []string `json:"audiences,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func (w *WebhookAuthenticatorProcessor) Process(ctx context.Context, req *WebhookAuthenticationRequest) (*AuthenticateInfo, error) {
	resp := &WebhookAuthenticationResponse{}
	if err := w.httpclient.Post("").JSON(req).Return(resp).Send(ctx); err != nil {
		return nil, err
	}
	if !resp.Authenticated {
		return nil, errors.NewUnauthorized(resp.Error)
	}
	info := &AuthenticateInfo{
		User:      resp.UserInfo,
		Audiences: resp.Audiences,
	}
	return info, nil
}
