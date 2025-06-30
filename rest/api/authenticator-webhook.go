package api

import (
	"context"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

type TokenWebhookAuthenticatorOptions struct {
	Server                string `json:"server,omitempty"`
	ProxyURL              string `json:"proxyURL,omitempty"`
	Token                 string `json:"token,omitempty"`
	Username              string `json:"username,omitempty"`
	Password              string `json:"password,omitempty"`
	CertFile              string `json:"certFile,omitempty"`
	KeyFile               string `json:"keyFile,omitempty"`
	CAFile                string `json:"caFile,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
	CookieName            string `json:"cookieName,omitempty" description:"cookie name for token, if not set, will not set cookie in response header"` // used to set cookie in response header
}

type TokenAuthenticationRequest struct {
	Token     string   `json:"token"`
	Username  string   `json:"username,omitempty"`
	Password  string   `json:"password,omitempty"`
	Audiences []string `json:"audiences,omitempty"`
}

type TokenAuthenticationResponse struct {
	Authenticated bool     `json:"authenticated"`
	UserInfo      UserInfo `json:"userInfo,omitempty"`
	Audiences     []string `json:"audiences,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func NewTokenWebhookAuthenticator(opts *TokenWebhookAuthenticatorOptions) (*TokenWebhookAuthenticator, error) {
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
	return &TokenWebhookAuthenticator{httpclient: cli}, nil
}

var _ TokenAuthenticator = &TokenWebhookAuthenticator{}

type TokenWebhookAuthenticator struct {
	httpclient *httpclient.Client
}

// Authenticate implements TokenAuthenticator.
func (t *TokenWebhookAuthenticator) Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error) {
	req := &TokenAuthenticationRequest{
		Token: token,
	}
	resp := &TokenAuthenticationResponse{}
	if err := t.httpclient.Post("").JSON(req).Return(resp).Send(ctx); err != nil {
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

var _ BasicAuthenticator = &BasicAuthWebhookAuthenticator{}

func NewBasicAuthWebhookAuthenticator(opts *TokenWebhookAuthenticatorOptions) (*BasicAuthWebhookAuthenticator, error) {
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
	return &BasicAuthWebhookAuthenticator{httpclient: cli}, nil
}

type BasicAuthWebhookAuthenticator struct {
	httpclient *httpclient.Client
}

// Authenticate implements TokenAuthenticator.
func (t *BasicAuthWebhookAuthenticator) Authenticate(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	req := &TokenAuthenticationRequest{
		Username: username,
		Password: password,
	}
	resp := &TokenAuthenticationResponse{}
	if err := t.httpclient.Post("").JSON(req).Return(resp).Send(ctx); err != nil {
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
