package api

import (
	"context"
	"net/http"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

// WebhookOptions is the basic options for webhook operations
type WebhookOptions struct {
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

func NewHttpClientFromWebhookOptions(opts *WebhookOptions) (*httpclient.Client, error) {
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
	return httpclient.NewClientFromConfig(context.Background(), config)
}

type WebhookAuthenticatorOptions struct {
	WebhookOptions `json:",inline"`
}

func NewWebhookAuthenticator(opts *WebhookAuthenticatorOptions) (*WebhookAuthenticator, error) {
	processor, err := NewWebhookAuthenticatorProcessor(&opts.WebhookOptions)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthenticator{Process: processor}, nil
}

var (
	_ Authenticator      = &WebhookAuthenticator{}
	_ TokenAuthenticator = &WebhookAuthenticator{}
	_ BasicAuthenticator = &WebhookAuthenticator{}
	_ SSHAuthenticator   = &WebhookAuthenticator{}
)

type WebhookAuthenticator struct {
	Process *WebhookAuthenticatorProcessor
}

var _ Authenticator = &WebhookAuthenticator{}

func (w *WebhookAuthenticator) Authenticate(wr http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	token := ExtractBearerTokenFromRequest(r)
	if token != "" {
		return w.AuthenticateToken(r.Context(), token)
	}
	username, password, ok := r.BasicAuth()
	if ok {
		return w.AuthenticateBasic(r.Context(), username, password)
	}
	return nil, ErrNotProvided
}

func (w *WebhookAuthenticator) AuthenticateToken(ctx context.Context, token string) (*AuthenticateInfo, error) {
	return w.Process.Process(ctx, &WebhookAuthenticationRequest{Token: token})
}

func (w *WebhookAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return w.Process.Process(ctx, &WebhookAuthenticationRequest{Username: username, Password: password})
}

func (w *WebhookAuthenticator) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error) {
	return w.Process.Process(ctx, &WebhookAuthenticationRequest{SSHCert: string(ssh.MarshalAuthorizedKey(pubkey))})
}

func NewWebhookAuthenticatorProcessor(opts *WebhookOptions) (*WebhookAuthenticatorProcessor, error) {
	cli, err := NewHttpClientFromWebhookOptions(opts)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthenticatorProcessor{httpclient: cli}, nil
}

type WebhookAuthenticatorProcessor struct {
	httpclient *httpclient.Client
}

type WebhookAuthenticationRequest struct {
	// token auth
	Token string `json:"token"`

	// basic auth
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// ssh auth
	// SSHCert is the SSH certificate used for authentication
	// It should be in OpenSSH certificate format
	// example:
	// -----BEGIN OPENSSH CERTIFICATE-----
	// ...
	// -----END OPENSSH CERTIFICATE-----
	SSHCert string `json:"sshCert,omitempty"`

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
