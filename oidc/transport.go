package oidc

import "net/http"

type clientCredentialsRoundTripper struct {
	client *Client
	base   http.RoundTripper
}

// NewClientCredentialsRoundTripper wraps base without creating an HTTP
// client. Token requests use each outgoing request's context.
func NewClientCredentialsRoundTripper(client *Client, base http.RoundTripper) http.RoundTripper {
	return &clientCredentialsRoundTripper{
		client: client,
		base:   base,
	}
}

// RoundTrip implements http.RoundTripper. The input request is cloned before
// its headers are changed.
func (t *clientCredentialsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	token, err := t.client.GetClientCredentialsToken(request.Context())
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
	return t.base.RoundTrip(clone)
}
