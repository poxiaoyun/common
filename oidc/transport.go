package oidc

import "net/http"

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
	token, err := t.source.Token(request.Context())
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
	return t.base.RoundTrip(clone)
}

// WrappedRoundTripper exposes the transport that owns the network and TLS
// configuration. WebSocket clients use it to reuse that configuration without
// executing this HTTP-only token injection layer.
func (t *clientCredentialsRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return t.base
}
