package httpclient

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"
)

// TransportWrapper composes runtime request behavior around a base transport.
type TransportWrapper func(http.RoundTripper) http.RoundTripper

// RequestAuthenticator prepares one outgoing request with its target
// authentication. Implementations may preserve or replace existing
// Authorization headers according to the identity they own.
type RequestAuthenticator interface {
	// AuthenticateRequest authenticates request without sending it.
	AuthenticateRequest(request *http.Request) error
}

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

// FindRequestAuthenticator returns the first request authenticator in a
// RoundTripper wrapping chain, or nil when the chain has none.
func FindRequestAuthenticator(transport http.RoundTripper) RequestAuthenticator {
	switch transport := transport.(type) {
	case RequestAuthenticator:
		return transport
	case WrappedRoundTripper:
		return FindRequestAuthenticator(transport.WrappedRoundTripper())
	default:
		return nil
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
	return AuthorizationRoundTripper{Authorization: "Bearer " + token, Transport: rt}
}

// AuthorizationRoundTripper adds a fixed Authorization value to cloned
// requests before passing them to Transport.
type AuthorizationRoundTripper struct {
	Authorization string
	Transport     http.RoundTripper
}

func (rt AuthorizationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if err := rt.AuthenticateRequest(clone); err != nil {
		return nil, err
	}
	return rt.Transport.RoundTrip(clone)
}

// AuthenticateRequest adds Authorization unless request already carries one.
func (rt AuthorizationRoundTripper) AuthenticateRequest(req *http.Request) error {
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", rt.Authorization)
	}
	return nil
}

// WrappedRoundTripper returns Transport.
func (rt AuthorizationRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.Transport
}

func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return AuthorizationRoundTripper{Authorization: "Basic " + credentials, Transport: rt}
}
