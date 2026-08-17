package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// RequestHeaderAuthenticatorOptions defines the trusted authentication
// assertion header.
type RequestHeaderAuthenticatorOptions struct {
	Header string `json:"header,omitempty" description:"the header containing the encoded authentication assertion"`
}

func NewDefaultRequestHeaderAuthenticatorOptions() *RequestHeaderAuthenticatorOptions {
	return &RequestHeaderAuthenticatorOptions{Header: "X-Remote-Authentication"}
}

// RequestHeaderCodec encodes and decodes authenticated identities in one
// base64url-encoded JSON header. It does not establish trust in the sender.
type RequestHeaderCodec struct {
	options RequestHeaderAuthenticatorOptions
}

func NewRequestHeaderCodec(opts *RequestHeaderAuthenticatorOptions) (*RequestHeaderCodec, error) {
	resolved := *NewDefaultRequestHeaderAuthenticatorOptions()
	if opts != nil && opts.Header != "" {
		resolved.Header = opts.Header
	}
	if !httpguts.ValidHeaderFieldName(resolved.Header) {
		return nil, fmt.Errorf("request header authenticator: invalid header %q", resolved.Header)
	}
	return &RequestHeaderCodec{options: resolved}, nil
}

// Encode replaces the configured assertion header with info. Validation is
// completed before req is mutated.
func (c *RequestHeaderCodec) Encode(req *http.Request, info AuthenticationInfo) error {
	if req == nil {
		return fmt.Errorf("request header codec: request is required")
	}
	if err := ValidateAuthenticationInfo(info); err != nil {
		return fmt.Errorf("request header codec: %w", err)
	}
	payload, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("request header codec: encode authentication: %w", err)
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	c.clear(req.Header)
	req.Header.Set(c.options.Header, base64.RawURLEncoding.EncodeToString(payload))
	return nil
}

// EncodeFromContext forwards the authentication installed by
// NewAuthenticationFilter.
func (c *RequestHeaderCodec) EncodeFromContext(ctx context.Context, req *http.Request) error {
	return c.Encode(req, AuthenticationFromContext(ctx))
}

// Decode reconstructs authentication from the assertion header without
// verifying its source.
func (c *RequestHeaderCodec) Decode(req *http.Request) (*AuthenticationInfo, error) {
	if req == nil {
		return nil, fmt.Errorf("request header codec: request is required")
	}
	values := c.headerValues(req.Header)
	if len(values) == 0 {
		return nil, ErrNotProvided
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("request header codec: %s must contain exactly one value", c.options.Header)
	}
	payload, err := base64.RawURLEncoding.DecodeString(values[0])
	if err != nil {
		return nil, fmt.Errorf("request header codec: decode authentication: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	info := &AuthenticationInfo{}
	if err := decoder.Decode(info); err != nil {
		return nil, fmt.Errorf("request header codec: decode authentication: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("request header codec: authentication must contain one JSON value")
	}
	if err := ValidateAuthenticationInfo(*info); err != nil {
		return nil, fmt.Errorf("request header codec: %w", err)
	}
	return info, nil
}

func (c *RequestHeaderCodec) headerValues(header http.Header) []string {
	var values []string
	for key, current := range header {
		if strings.EqualFold(key, c.options.Header) {
			values = append(values, current...)
		}
	}
	return values
}

func (c *RequestHeaderCodec) hasAuthenticationHeader(header http.Header) bool {
	return len(c.headerValues(header)) != 0
}

func (c *RequestHeaderCodec) clear(header http.Header) {
	for key := range header {
		if strings.EqualFold(key, c.options.Header) {
			delete(header, key)
		}
	}
}

// RequestHeaderTrustVerifier verifies that an assertion came from a trusted
// proxy, for example by checking a mutually authenticated TLS peer.
type RequestHeaderTrustVerifier interface {
	VerifyRequest(*http.Request) error
}

type RequestHeaderTrustFunc func(*http.Request) error

var _ RequestHeaderTrustVerifier = RequestHeaderTrustFunc(nil)

func (f RequestHeaderTrustFunc) VerifyRequest(req *http.Request) error {
	if f == nil {
		return fmt.Errorf("request header trust verifier is nil")
	}
	return f(req)
}

// RequestHeaderAuthenticator authenticates assertions from a trusted proxy.
// The proxy must remove client-supplied assertion headers.
type RequestHeaderAuthenticator struct {
	codec    *RequestHeaderCodec
	verifier RequestHeaderTrustVerifier
}

var _ Authenticator = (*RequestHeaderAuthenticator)(nil)

func NewRequestHeaderAuthenticator(codec *RequestHeaderCodec, verifier RequestHeaderTrustVerifier) (*RequestHeaderAuthenticator, error) {
	if codec == nil {
		return nil, fmt.Errorf("request header authenticator: codec is required")
	}
	if isNilVerifier(verifier) {
		return nil, fmt.Errorf("request header authenticator: trust verifier is required")
	}
	return &RequestHeaderAuthenticator{codec: codec, verifier: verifier}, nil
}

func isNilVerifier(verifier RequestHeaderTrustVerifier) bool {
	if verifier == nil {
		return true
	}
	value := reflect.ValueOf(verifier)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (a *RequestHeaderAuthenticator) Authenticate(_ http.ResponseWriter, req *http.Request) (*AuthenticationInfo, error) {
	if req == nil || !a.codec.hasAuthenticationHeader(req.Header) {
		return nil, ErrNotProvided
	}
	if err := a.verifier.VerifyRequest(req); err != nil {
		return nil, fmt.Errorf("request header authenticator: verify trusted proxy: %w", err)
	}
	return a.codec.Decode(req)
}

type authenticationProxyRoundTripper struct {
	codec     *RequestHeaderCodec
	info      AuthenticationInfo
	fromCtx   bool
	transport http.RoundTripper
}

var _ http.RoundTripper = (*authenticationProxyRoundTripper)(nil)

// NewAuthenticationProxyRoundTripper forwards fixed authentication information.
func NewAuthenticationProxyRoundTripper(codec *RequestHeaderCodec, info AuthenticationInfo, transport http.RoundTripper) (http.RoundTripper, error) {
	if codec == nil {
		return nil, fmt.Errorf("authentication proxy round tripper: codec is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("authentication proxy round tripper: transport is required")
	}
	if err := ValidateAuthenticationInfo(info); err != nil {
		return nil, fmt.Errorf("authentication proxy round tripper: %w", err)
	}
	return &authenticationProxyRoundTripper{codec: codec, info: info, transport: transport}, nil
}

// NewContextAuthenticationProxyRoundTripper forwards authentication stored in
// each request context by NewAuthenticationFilter.
func NewContextAuthenticationProxyRoundTripper(codec *RequestHeaderCodec, transport http.RoundTripper) (http.RoundTripper, error) {
	if codec == nil {
		return nil, fmt.Errorf("authentication proxy round tripper: codec is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("authentication proxy round tripper: transport is required")
	}
	return &authenticationProxyRoundTripper{codec: codec, fromCtx: true, transport: transport}, nil
}

func (rt *authenticationProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("authentication proxy round tripper: request is required")
	}
	info := rt.info
	if rt.fromCtx {
		info = AuthenticationFromContext(req.Context())
	}
	clone := req.Clone(req.Context())
	if err := rt.codec.Encode(clone, info); err != nil {
		return nil, err
	}
	return rt.transport.RoundTrip(clone)
}

func (rt *authenticationProxyRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.transport
}
