package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

// TransportWrapper composes runtime request behavior around a base transport.
type TransportWrapper func(http.RoundTripper) http.RoundTripper

// WrappedRoundTripper exposes the next transport in a wrapping chain.
type WrappedRoundTripper interface {
	// WrappedRoundTripper returns the transport wrapped by this implementation.
	WrappedRoundTripper() http.RoundTripper
}

// TLSClientConfigHolder exposes the TLS configuration used by a RoundTripper.
type TLSClientConfigHolder interface {
	// TLSClientConfig returns the transport's effective TLS configuration.
	TLSClientConfig() *tls.Config
}

// TLSClientConfig returns the TLS configuration from a known or wrapped
// RoundTripper.
func TLSClientConfig(transport http.RoundTripper) (*tls.Config, error) {
	switch transport := transport.(type) {
	case *http.Transport:
		return transport.TLSClientConfig, nil
	case TLSClientConfigHolder:
		return transport.TLSClientConfig(), nil
	case WrappedRoundTripper:
		return TLSClientConfig(transport.WrappedRoundTripper())
	default:
		return nil, fmt.Errorf("round tripper %T does not expose a TLS client config", transport)
	}
}

const MaxidleConnsPerHost = 25

func NewDefaultHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: MaxidleConnsPerHost,
	}
}

func NewBearerTokenRoundTripper(token string, rt http.RoundTripper) http.RoundTripper {
	return &BearerTokenRoundTripper{token: token, transport: rt}
}

func NewBearerTokenFuncRoundTripper(tokenFunc func(r *http.Request) (string, error), rt http.RoundTripper) (http.RoundTripper, error) {
	if tokenFunc == nil {
		return nil, fmt.Errorf("tokenFunc is required")
	}
	return &BearerTokenRoundTripper{tokenFunc: tokenFunc, transport: rt}, nil
}

type BearerTokenRoundTripper struct {
	token     string
	tokenFunc func(r *http.Request) (string, error)
	transport http.RoundTripper
}

func (rt *BearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the request already has an Authorization header, we assume it is already authenticated.
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.transport.RoundTrip(req)
	}
	token := rt.token
	if rt.tokenFunc != nil {
		dynamicToken, err := rt.tokenFunc(req)
		if err != nil {
			return nil, err
		}
		token = dynamicToken
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.transport.RoundTrip(req)
}

func (rt *BearerTokenRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.transport }

type BasicAuthRoundTripper struct {
	username  string
	password  string
	transport http.RoundTripper
}

func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &BasicAuthRoundTripper{username: username, password: password, transport: rt}
}

func (rt *BasicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.transport.RoundTrip(req)
	}
	req.SetBasicAuth(rt.username, rt.password)
	return rt.transport.RoundTrip(req)
}

func (rt *BasicAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.transport }
