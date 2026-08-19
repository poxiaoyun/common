package api

import (
	"context"
	"net/http"
	"slices"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

// WebhookTransportWrapper decorates the transport created from WebhookOptions.
type WebhookTransportWrapper func(http.RoundTripper) http.RoundTripper

// WebhookOptions is the basic options for webhook operations
type WebhookOptions struct {
	Server                string `json:"server,omitempty"`
	ProxyURL              string `json:"proxyURL,omitempty"`
	Token                 string `json:"token,omitempty" config:"token,sensitive"`
	Username              string `json:"username,omitempty"`
	Password              string `json:"password,omitempty" config:"password,sensitive"`
	CertFile              string `json:"certFile,omitempty"`
	KeyFile               string `json:"keyFile,omitempty"`
	CAFile                string `json:"caFile,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
}

// NewHTTPClientFromWebhookOptions returns a client configured for a webhook.
func NewHTTPClientFromWebhookOptions(opts *WebhookOptions) (*httpclient.Client, error) {
	return newHTTPClientFromWebhookOptions(context.Background(), opts, nil)
}

func newHTTPClientFromWebhookOptions(ctx context.Context, opts *WebhookOptions, wrapper WebhookTransportWrapper) (*httpclient.Client, error) {
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
	client, err := httpclient.NewClientFromConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if wrapper != nil {
		client.RoundTripper = wrapper(client.RoundTripper)
	}
	return client, nil
}

type WebhookAuthenticatorOptions struct {
	WebhookOptions `json:",inline"`
	// Audiences delegates audience validation to the authentication review
	// service. Resource Servers that validate OAuth tokens locally leave it empty.
	Audiences []string `json:"audiences,omitempty" description:"Audiences requested when reviewing bearer tokens"`
}

func NewWebhookAuthenticator(opts *WebhookAuthenticatorOptions) (*WebhookAuthenticator, error) {
	return NewWebhookAuthenticatorWithTransport(opts, nil)
}

// NewWebhookAuthenticatorWithTransport creates a Review authenticator whose
// requests use wrapper around the WebhookOptions transport.
func NewWebhookAuthenticatorWithTransport(opts *WebhookAuthenticatorOptions, wrapper WebhookTransportWrapper) (*WebhookAuthenticator, error) {
	client, err := newHTTPClientFromWebhookOptions(context.Background(), &opts.WebhookOptions, wrapper)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthenticator{
		Process:   &WebhookAuthenticatorProcessor{httpclient: client},
		Audiences: opts.Audiences,
	}, nil
}

var (
	_ Authenticator      = &WebhookAuthenticator{}
	_ TokenAuthenticator = &WebhookAuthenticator{}
	_ BasicAuthenticator = &WebhookAuthenticator{}
	_ SSHAuthenticator   = &WebhookAuthenticator{}
)

type WebhookAuthenticator struct {
	Process   *WebhookAuthenticatorProcessor
	Audiences []string
}

var _ Authenticator = &WebhookAuthenticator{}

func (w *WebhookAuthenticator) Authenticate(wr http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
	token, provided := extractBearerTokenFromRequest(r)
	if provided {
		if token == "" {
			return nil, errors.NewUnauthorized("bearer token is empty")
		}
		return w.AuthenticateToken(r.Context(), token)
	}
	username, password, ok := r.BasicAuth()
	if ok {
		return w.AuthenticateBasic(r.Context(), username, password)
	}
	return nil, ErrNotProvided
}

func (w *WebhookAuthenticator) AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error) {
	return w.Process.Process(ctx, &AuthenticationReviewSpec{Token: token, Audiences: w.Audiences})
}

func (w *WebhookAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	return w.Process.Process(ctx, &AuthenticationReviewSpec{Username: username, Password: password})
}

func (w *WebhookAuthenticator) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticationInfo, error) {
	return w.Process.Process(ctx, &AuthenticationReviewSpec{SSHPublicKey: string(ssh.MarshalAuthorizedKey(pubkey))})
}

func NewWebhookAuthenticatorProcessor(opts *WebhookOptions) (*WebhookAuthenticatorProcessor, error) {
	cli, err := NewHTTPClientFromWebhookOptions(opts)
	if err != nil {
		return nil, err
	}
	return &WebhookAuthenticatorProcessor{httpclient: cli}, nil
}

type WebhookAuthenticatorProcessor struct {
	httpclient *httpclient.Client
}

func (w *WebhookAuthenticatorProcessor) Process(ctx context.Context, spec *AuthenticationReviewSpec) (*AuthenticationInfo, error) {
	review := &AuthenticationReview{Spec: spec}
	response := &AuthenticationReview{}
	if err := w.httpclient.Post("").JSON(review).Return(response).Send(ctx); err != nil {
		return nil, err
	}
	if response.Status == nil {
		return nil, errors.NewUnauthorized("authentication review returned no status")
	}
	if !response.Status.Authenticated {
		return nil, errors.NewUnauthorized(response.Status.Error)
	}
	if response.Status.Authentication == nil {
		return nil, errors.NewUnauthorized("authentication review returned no authentication")
	}
	if len(spec.Audiences) > 0 && !slices.ContainsFunc(response.Status.Audiences, func(audience string) bool {
		return slices.Contains(spec.Audiences, audience)
	}) {
		return nil, errors.NewUnauthorized("authentication review returned no compatible audience")
	}
	if err := ValidateAuthenticationInfo(*response.Status.Authentication); err != nil {
		return nil, errors.NewUnauthorized(err.Error())
	}
	return response.Status.Authentication, nil
}
