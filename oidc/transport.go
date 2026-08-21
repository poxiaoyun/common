package oidc

import (
	"net/http"

	"xiaoshiai.cn/common/httpclient"
)

type clientCredentialsRoundTripper struct {
	source *ClientCredentialsTokenSource
	base   http.RoundTripper
}

// NewClientCredentialsRoundTripper wraps base with one target-bound token
// source. Token requests use each outgoing request's context.
func NewClientCredentialsRoundTripper(source *ClientCredentialsTokenSource, base http.RoundTripper) http.RoundTripper {
	return &clientCredentialsRoundTripper{
		source: source,
		base:   base,
	}
}

// RoundTrip implements http.RoundTripper. The input request is cloned before
// its headers are changed.
func (t *clientCredentialsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := httpclient.CloneRequest(request)
	if err := t.AuthenticateRequest(clone); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(clone)
}

// AuthenticateRequest sets a target-bound Client Credentials access token on
// request, replacing any caller-supplied Authorization header.
func (t *clientCredentialsRoundTripper) AuthenticateRequest(request *http.Request) error {
	token, err := t.source.Token(request.Context())
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
	return nil
}

// WrappedRoundTripper exposes the transport that owns the network and TLS
// configuration. WebSocket clients use it to reuse that configuration without
// executing this HTTP-only token injection layer.
func (t *clientCredentialsRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return t.base
}

var _ httpclient.RequestAuthenticator = (*clientCredentialsRoundTripper)(nil)
