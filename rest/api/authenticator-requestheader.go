package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

// RequestHeaderAuthenticatorOptions defines the wire format used to forward an
// authenticated identity between trusted services.
type RequestHeaderAuthenticatorOptions struct {
	NameHeader          string `json:"nameHeader,omitempty" description:"the header containing the user name"`
	UserIDHeader        string `json:"userIDHeader,omitempty" description:"the header containing the user ID"`
	EmailHeader         string `json:"emailHeader,omitempty" description:"the header containing the user email"`
	EmailVerifiedHeader string `json:"emailVerifiedHeader,omitempty" description:"the header indicating whether the user email is verified"`
	GroupsHeader        string `json:"groupsHeader,omitempty" description:"the header containing user groups"`
	AudiencesHeader     string `json:"audiencesHeader,omitempty" description:"the header containing authenticated audiences"`
	ExtraHeaderPrefix   string `json:"extraHeaderPrefix,omitempty" description:"the prefix used for extra attribute headers"`
}

func NewDefaultRequestHeaderAuthenticatorOptions() *RequestHeaderAuthenticatorOptions {
	return &RequestHeaderAuthenticatorOptions{
		NameHeader:          "X-Remote-User",
		UserIDHeader:        "X-Remote-User-ID",
		EmailHeader:         "X-Remote-User-Email",
		EmailVerifiedHeader: "X-Remote-User-Email-Verified",
		GroupsHeader:        "X-Remote-Group",
		AudiencesHeader:     "X-Remote-Audience",
		ExtraHeaderPrefix:   "X-Remote-Extra-",
	}
}

// RequestHeaderCodec encodes and decodes authenticated identities in HTTP
// headers. It does not establish that the headers came from a trusted source;
// use RequestHeaderAuthenticator when authenticating an inbound request.
type RequestHeaderCodec struct {
	options RequestHeaderAuthenticatorOptions
}

func NewRequestHeaderCodec(opts *RequestHeaderAuthenticatorOptions) (*RequestHeaderCodec, error) {
	resolved := *NewDefaultRequestHeaderAuthenticatorOptions()
	if opts != nil {
		fillRequestHeaderOptions(&resolved, *opts)
	}
	if err := validateRequestHeaderOptions(resolved); err != nil {
		return nil, err
	}
	return &RequestHeaderCodec{options: resolved}, nil
}

func fillRequestHeaderOptions(dst *RequestHeaderAuthenticatorOptions, src RequestHeaderAuthenticatorOptions) {
	if src.NameHeader != "" {
		dst.NameHeader = src.NameHeader
	}
	if src.UserIDHeader != "" {
		dst.UserIDHeader = src.UserIDHeader
	}
	if src.EmailHeader != "" {
		dst.EmailHeader = src.EmailHeader
	}
	if src.EmailVerifiedHeader != "" {
		dst.EmailVerifiedHeader = src.EmailVerifiedHeader
	}
	if src.GroupsHeader != "" {
		dst.GroupsHeader = src.GroupsHeader
	}
	if src.AudiencesHeader != "" {
		dst.AudiencesHeader = src.AudiencesHeader
	}
	if src.ExtraHeaderPrefix != "" {
		dst.ExtraHeaderPrefix = src.ExtraHeaderPrefix
	}
}

func validateRequestHeaderOptions(opts RequestHeaderAuthenticatorOptions) error {
	names := []struct {
		field string
		value string
	}{
		{"nameHeader", opts.NameHeader},
		{"userIDHeader", opts.UserIDHeader},
		{"emailHeader", opts.EmailHeader},
		{"emailVerifiedHeader", opts.EmailVerifiedHeader},
		{"groupsHeader", opts.GroupsHeader},
		{"audiencesHeader", opts.AudiencesHeader},
	}
	seen := make(map[string]string, len(names))
	for _, name := range names {
		if !httpguts.ValidHeaderFieldName(name.value) {
			return fmt.Errorf("request header authenticator: invalid %s %q", name.field, name.value)
		}
		canonical := strings.ToLower(name.value)
		if previous, ok := seen[canonical]; ok {
			return fmt.Errorf("request header authenticator: %s conflicts with %s", name.field, previous)
		}
		if hasPrefixFold(name.value, opts.ExtraHeaderPrefix) {
			return fmt.Errorf("request header authenticator: %s falls within extraHeaderPrefix", name.field)
		}
		seen[canonical] = name.field
	}
	if !httpguts.ValidHeaderFieldName(opts.ExtraHeaderPrefix) {
		return fmt.Errorf("request header authenticator: invalid extraHeaderPrefix %q", opts.ExtraHeaderPrefix)
	}
	return nil
}

// Encode replaces all configured identity headers on req with info. Validation
// is completed before req is mutated.
func (c *RequestHeaderCodec) Encode(req *http.Request, info AuthenticateInfo) error {
	if req == nil {
		return fmt.Errorf("request header codec: request is required")
	}
	encodedExtra, err := validateAndEncodeIdentity(info)
	if err != nil {
		return err
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	c.clear(req.Header)
	opts := c.options
	req.Header.Set(opts.NameHeader, info.User.Name)
	if info.User.ID != "" {
		req.Header.Set(opts.UserIDHeader, info.User.ID)
	}
	if info.User.Email != "" {
		req.Header.Set(opts.EmailHeader, info.User.Email)
	}
	req.Header.Set(opts.EmailVerifiedHeader, strconv.FormatBool(info.User.EmailVerified))
	for _, group := range info.User.Groups {
		req.Header.Add(opts.GroupsHeader, group)
	}
	for _, audience := range info.Audiences {
		req.Header.Add(opts.AudiencesHeader, audience)
	}
	for key, values := range encodedExtra {
		for _, value := range values {
			req.Header.Add(opts.ExtraHeaderPrefix+key, value)
		}
	}
	return nil
}

// EncodeFromContext forwards the identity installed by NewAuthenticateFilter.
func (c *RequestHeaderCodec) EncodeFromContext(ctx context.Context, req *http.Request) error {
	return c.Encode(req, AuthenticateFromContext(ctx))
}

func validateAndEncodeIdentity(info AuthenticateInfo) (map[string][]string, error) {
	if info.User.Name == "" {
		return nil, fmt.Errorf("request header codec: user name is required")
	}
	values := []struct {
		field string
		value string
	}{
		{"user name", info.User.Name},
		{"user ID", info.User.ID},
		{"user email", info.User.Email},
	}
	for _, value := range values {
		if !httpguts.ValidHeaderFieldValue(value.value) {
			return nil, fmt.Errorf("request header codec: invalid %s", value.field)
		}
	}
	for _, value := range info.User.Groups {
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("request header codec: invalid group")
		}
	}
	for _, value := range info.Audiences {
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("request header codec: invalid audience")
		}
	}
	encoded := make(map[string][]string, len(info.User.Extra))
	seen := make(map[string]string, len(info.User.Extra))
	for key, extraValues := range info.User.Extra {
		canonical := strings.ToLower(key)
		if canonical == "" || !utf8.ValidString(canonical) {
			return nil, fmt.Errorf("request header codec: invalid empty or non-UTF-8 extra key")
		}
		if previous, ok := seen[canonical]; ok {
			return nil, fmt.Errorf("request header codec: extra keys %q and %q conflict", previous, key)
		}
		for _, value := range extraValues {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, fmt.Errorf("request header codec: invalid value for extra key %q", key)
			}
		}
		seen[canonical] = key
		encoded[headerKeyEscape(canonical)] = append([]string(nil), extraValues...)
	}
	return encoded, nil
}

// Decode reconstructs an identity from headers without verifying their source.
func (c *RequestHeaderCodec) Decode(req *http.Request) (*AuthenticateInfo, error) {
	if req == nil {
		return nil, fmt.Errorf("request header codec: request is required")
	}
	if !c.hasIdentityHeaders(req.Header) {
		return nil, ErrNotProvided
	}
	opts := c.options
	name, present, err := singleHeader(req.Header, opts.NameHeader)
	if err != nil {
		return nil, err
	}
	if !present || name == "" {
		return nil, fmt.Errorf("request header codec: user name is required")
	}
	id, _, err := singleHeader(req.Header, opts.UserIDHeader)
	if err != nil {
		return nil, err
	}
	email, _, err := singleHeader(req.Header, opts.EmailHeader)
	if err != nil {
		return nil, err
	}
	emailVerifiedValue, hasEmailVerified, err := singleHeader(req.Header, opts.EmailVerifiedHeader)
	if err != nil {
		return nil, err
	}
	emailVerified := false
	if hasEmailVerified {
		switch emailVerifiedValue {
		case "true":
			emailVerified = true
		case "false":
		default:
			return nil, fmt.Errorf("request header codec: %s must be true or false", opts.EmailVerifiedHeader)
		}
	}
	groups, _ := headerValues(req.Header, opts.GroupsHeader)
	audiences, _ := headerValues(req.Header, opts.AudiencesHeader)
	identityValues := append([]string{name, id, email}, groups...)
	identityValues = append(identityValues, audiences...)
	for _, value := range identityValues {
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("request header codec: invalid identity header value")
		}
	}
	extra := make(map[string][]string)
	seenExtraHeaders := make(map[string]string)
	for headerName, values := range req.Header {
		if !hasPrefixFold(headerName, opts.ExtraHeaderPrefix) {
			continue
		}
		encodedKey := headerName[len(opts.ExtraHeaderPrefix):]
		key, err := decodeExtraKey(encodedKey)
		if err != nil {
			return nil, err
		}
		if previous, ok := seenExtraHeaders[key]; ok {
			return nil, fmt.Errorf("request header codec: extra headers %q and %q conflict", previous, headerName)
		}
		for _, value := range values {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, fmt.Errorf("request header codec: invalid value for extra key %q", key)
			}
		}
		seenExtraHeaders[key] = headerName
		extra[key] = append([]string(nil), values...)
	}
	return &AuthenticateInfo{
		Audiences: append([]string(nil), audiences...),
		User: UserInfo{
			ID:            id,
			Name:          name,
			Email:         email,
			EmailVerified: emailVerified,
			Groups:        append([]string(nil), groups...),
			Extra:         extra,
		},
	}, nil
}

func singleHeader(header http.Header, name string) (string, bool, error) {
	values, present := headerValues(header, name)
	if !present {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fmt.Errorf("request header codec: %s must contain exactly one value", name)
	}
	return values[0], true, nil
}

func headerValues(header http.Header, name string) ([]string, bool) {
	var values []string
	present := false
	for key, current := range header {
		if strings.EqualFold(key, name) {
			present = true
			values = append(values, current...)
		}
	}
	return values, present
}

func (c *RequestHeaderCodec) hasIdentityHeaders(header http.Header) bool {
	opts := c.options
	for key := range header {
		if strings.EqualFold(key, opts.NameHeader) ||
			strings.EqualFold(key, opts.UserIDHeader) ||
			strings.EqualFold(key, opts.EmailHeader) ||
			strings.EqualFold(key, opts.EmailVerifiedHeader) ||
			strings.EqualFold(key, opts.GroupsHeader) ||
			strings.EqualFold(key, opts.AudiencesHeader) ||
			hasPrefixFold(key, opts.ExtraHeaderPrefix) {
			return true
		}
	}
	return false
}

func (c *RequestHeaderCodec) clear(header http.Header) {
	opts := c.options
	for key := range header {
		if strings.EqualFold(key, opts.NameHeader) ||
			strings.EqualFold(key, opts.UserIDHeader) ||
			strings.EqualFold(key, opts.EmailHeader) ||
			strings.EqualFold(key, opts.EmailVerifiedHeader) ||
			strings.EqualFold(key, opts.GroupsHeader) ||
			strings.EqualFold(key, opts.AudiencesHeader) ||
			hasPrefixFold(key, opts.ExtraHeaderPrefix) {
			delete(header, key)
		}
	}
}

func hasPrefixFold(value, prefix string) bool {
	return len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)
}

func decodeExtraKey(encodedKey string) (string, error) {
	if encodedKey == "" {
		return "", fmt.Errorf("request header codec: extra key is empty")
	}
	key, err := url.PathUnescape(strings.ToLower(encodedKey))
	if err != nil {
		return "", fmt.Errorf("request header codec: invalid escaped extra key %q: %w", encodedKey, err)
	}
	if key == "" || !utf8.ValidString(key) {
		return "", fmt.Errorf("request header codec: invalid empty or non-UTF-8 extra key")
	}
	return key, nil
}

func headerKeyEscape(key string) string {
	var buf strings.Builder
	for i := 0; i < len(key); i++ {
		b := key[i]
		if !isLegalHeaderKeyByte(b) || b == '%' {
			fmt.Fprintf(&buf, "%%%02X", b)
			continue
		}
		buf.WriteByte(b)
	}
	return buf.String()
}

func isLegalHeaderKeyByte(b byte) bool {
	return b < utf8.RuneSelf && httpguts.ValidHeaderFieldName(string([]byte{b}))
}

// RequestHeaderTrustVerifier verifies that identity headers came from a
// trusted proxy, for example by checking a mutually authenticated TLS peer.
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

// RequestHeaderAuthenticator authenticates identities asserted by a trusted
// proxy. Callers must configure the proxy boundary to remove client-supplied
// identity headers and use verifier to validate the immediate peer. A typical
// verifier checks req.TLS.VerifiedChains for the expected proxy certificate;
// network isolation alone can also be asserted with an explicit verifier.
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

func (a *RequestHeaderAuthenticator) Authenticate(_ http.ResponseWriter, req *http.Request) (*AuthenticateInfo, error) {
	if req == nil || !a.codec.hasIdentityHeaders(req.Header) {
		return nil, ErrNotProvided
	}
	if err := a.verifier.VerifyRequest(req); err != nil {
		return nil, fmt.Errorf("request header authenticator: verify trusted proxy: %w", err)
	}
	return a.codec.Decode(req)
}

type authProxyRoundTripper struct {
	codec     *RequestHeaderCodec
	info      AuthenticateInfo
	fromCtx   bool
	transport http.RoundTripper
}

var _ http.RoundTripper = (*authProxyRoundTripper)(nil)

// NewAuthProxyRoundTripper forwards a fixed authenticated identity. It replaces
// the former primitive identity transport from the httpclient package.
func NewAuthProxyRoundTripper(codec *RequestHeaderCodec, info AuthenticateInfo, transport http.RoundTripper) (http.RoundTripper, error) {
	if codec == nil {
		return nil, fmt.Errorf("auth proxy round tripper: codec is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("auth proxy round tripper: transport is required")
	}
	if _, err := validateAndEncodeIdentity(info); err != nil {
		return nil, err
	}
	return &authProxyRoundTripper{codec: codec, info: cloneAuthenticateInfo(info), transport: transport}, nil
}

// NewContextAuthProxyRoundTripper forwards the identity stored in each
// request's context by NewAuthenticateFilter.
func NewContextAuthProxyRoundTripper(codec *RequestHeaderCodec, transport http.RoundTripper) (http.RoundTripper, error) {
	if codec == nil {
		return nil, fmt.Errorf("auth proxy round tripper: codec is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("auth proxy round tripper: transport is required")
	}
	return &authProxyRoundTripper{codec: codec, fromCtx: true, transport: transport}, nil
}

func (rt *authProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("auth proxy round tripper: request is required")
	}
	info := rt.info
	if rt.fromCtx {
		info = AuthenticateFromContext(req.Context())
	}
	clone := req.Clone(req.Context())
	if err := rt.codec.Encode(clone, info); err != nil {
		return nil, err
	}
	return rt.transport.RoundTrip(clone)
}

func (rt *authProxyRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.transport }

func cloneAuthenticateInfo(info AuthenticateInfo) AuthenticateInfo {
	cloned := info
	cloned.Audiences = append([]string(nil), info.Audiences...)
	cloned.User.Groups = append([]string(nil), info.User.Groups...)
	cloned.User.Extra = make(map[string][]string, len(info.User.Extra))
	for key, values := range info.User.Extra {
		cloned.User.Extra[key] = append([]string(nil), values...)
	}
	return cloned
}
