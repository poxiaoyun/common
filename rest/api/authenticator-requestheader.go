package api

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/httpclient"
)

// RequestHeaderAuthenticatorOptions defines the wire format used to forward
// authentication information between trusted services.
type RequestHeaderAuthenticatorOptions struct {
	SubjectTypeHeader        string `json:"subjectTypeHeader,omitempty" description:"the header containing the subject type"`
	NameHeader               string `json:"nameHeader,omitempty" description:"the header containing the subject name"`
	UserIDHeader             string `json:"userIDHeader,omitempty" description:"the header containing the stable subject ID"`
	DisplayNameHeader        string `json:"displayNameHeader,omitempty" description:"the header containing the subject display name"`
	EmailHeader              string `json:"emailHeader,omitempty" description:"the header containing the subject email"`
	EmailVerifiedHeader      string `json:"emailVerifiedHeader,omitempty" description:"the header indicating whether the subject email is verified"`
	GroupsHeader             string `json:"groupsHeader,omitempty" description:"the header containing subject groups"`
	ActorTypeHeader          string `json:"actorTypeHeader,omitempty" description:"the header containing the actor type"`
	ActorHeader              string `json:"actorHeader,omitempty" description:"the header containing the stable actor ID"`
	ActorNameHeader          string `json:"actorNameHeader,omitempty" description:"the header containing the actor name"`
	ActorDisplayNameHeader   string `json:"actorDisplayNameHeader,omitempty" description:"the header containing the actor display name"`
	ActorEmailHeader         string `json:"actorEmailHeader,omitempty" description:"the header containing the actor email"`
	ActorEmailVerifiedHeader string `json:"actorEmailVerifiedHeader,omitempty" description:"the header indicating whether the actor email is verified"`
	ActorGroupsHeader        string `json:"actorGroupsHeader,omitempty" description:"the header containing actor groups"`
	AccessHeader             string `json:"accessHeader,omitempty" description:"the header indicating OAuth access-token authentication"`
	AudiencesHeader          string `json:"audiencesHeader,omitempty" description:"the header containing access-token audiences"`
	ScopesHeader             string `json:"scopesHeader,omitempty" description:"the header containing access-token scopes"`
}

func NewDefaultRequestHeaderAuthenticatorOptions() *RequestHeaderAuthenticatorOptions {
	return &RequestHeaderAuthenticatorOptions{
		SubjectTypeHeader:        "X-Remote-Extra-Subject-Type",
		NameHeader:               "X-Remote-User",
		UserIDHeader:             "X-Remote-Uid",
		DisplayNameHeader:        "X-Remote-Extra-Display-Name",
		EmailHeader:              "X-Remote-Extra-Email",
		EmailVerifiedHeader:      "X-Remote-Extra-Email-Verified",
		GroupsHeader:             "X-Remote-Group",
		ActorTypeHeader:          "X-Remote-Extra-Actor-Type",
		ActorHeader:              "X-Remote-Extra-Actor",
		ActorNameHeader:          "X-Remote-Extra-Actor-Name",
		ActorDisplayNameHeader:   "X-Remote-Extra-Actor-Display-Name",
		ActorEmailHeader:         "X-Remote-Extra-Actor-Email",
		ActorEmailVerifiedHeader: "X-Remote-Extra-Actor-Email-Verified",
		ActorGroupsHeader:        "X-Remote-Extra-Actor-Group",
		AccessHeader:             "X-Remote-Extra-Access",
		AudiencesHeader:          "X-Remote-Extra-Audience",
		ScopesHeader:             "X-Remote-Extra-Scopes",
	}
}

// RequestHeaderCodec maps Authentication to and from HTTP headers. It
// does not establish trust in the sender or validate authentication data.
type RequestHeaderCodec interface {
	Encode(*http.Request, Authentication)
	EncodeFromContext(context.Context, *http.Request)
	Decode(*http.Request) (*Authentication, error)
	Has(http.Header) bool
	Clear(http.Header)
}

type requestHeaderCodec struct {
	options RequestHeaderAuthenticatorOptions
}

var _ RequestHeaderCodec = (*requestHeaderCodec)(nil)

func NewRequestHeaderCodec(opts *RequestHeaderAuthenticatorOptions) RequestHeaderCodec {
	resolved := *NewDefaultRequestHeaderAuthenticatorOptions()
	if opts != nil {
		fields := []struct {
			dst *string
			src string
		}{
			{&resolved.SubjectTypeHeader, opts.SubjectTypeHeader},
			{&resolved.NameHeader, opts.NameHeader},
			{&resolved.UserIDHeader, opts.UserIDHeader},
			{&resolved.DisplayNameHeader, opts.DisplayNameHeader},
			{&resolved.EmailHeader, opts.EmailHeader},
			{&resolved.EmailVerifiedHeader, opts.EmailVerifiedHeader},
			{&resolved.GroupsHeader, opts.GroupsHeader},
			{&resolved.ActorTypeHeader, opts.ActorTypeHeader},
			{&resolved.ActorHeader, opts.ActorHeader},
			{&resolved.ActorNameHeader, opts.ActorNameHeader},
			{&resolved.ActorDisplayNameHeader, opts.ActorDisplayNameHeader},
			{&resolved.ActorEmailHeader, opts.ActorEmailHeader},
			{&resolved.ActorEmailVerifiedHeader, opts.ActorEmailVerifiedHeader},
			{&resolved.ActorGroupsHeader, opts.ActorGroupsHeader},
			{&resolved.AccessHeader, opts.AccessHeader},
			{&resolved.AudiencesHeader, opts.AudiencesHeader},
			{&resolved.ScopesHeader, opts.ScopesHeader},
		}
		for _, field := range fields {
			if field.src != "" {
				*field.dst = field.src
			}
		}
	}
	return &requestHeaderCodec{options: resolved}
}

// Encode replaces every configured authentication header with info.
func (c *requestHeaderCodec) Encode(req *http.Request, info Authentication) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	c.Clear(req.Header)
	c.encodeSubject(req.Header, info.Subject, false)
	if info.Actor != nil {
		c.encodeSubject(req.Header, *info.Actor, true)
	}
	if info.Token != nil {
		req.Header.Set(c.options.AccessHeader, "oauth2")
		addHeaderValues(req.Header, c.options.AudiencesHeader, info.Token.Audiences)
		addHeaderValues(req.Header, c.options.ScopesHeader, info.Token.Scopes)
	}
}

func (c *requestHeaderCodec) encodeSubject(header http.Header, subject Subject, actor bool) {
	if actor {
		setHeaderIfNotEmpty(header, c.options.ActorTypeHeader, string(subject.Type))
		header.Set(c.options.ActorHeader, subject.ID)
		setHeaderIfNotEmpty(header, c.options.ActorNameHeader, subject.Name)
		setHeaderIfNotEmpty(header, c.options.ActorDisplayNameHeader, subject.DisplayName)
		setHeaderIfNotEmpty(header, c.options.ActorEmailHeader, subject.Email)
		if subject.EmailVerified {
			header.Set(c.options.ActorEmailVerifiedHeader, strconv.FormatBool(subject.EmailVerified))
		}
		addHeaderValues(header, c.options.ActorGroupsHeader, subject.Groups)
		return
	}
	setHeaderIfNotEmpty(header, c.options.SubjectTypeHeader, string(subject.Type))
	header.Set(c.options.UserIDHeader, subject.ID)
	setHeaderIfNotEmpty(header, c.options.NameHeader, subject.Name)
	setHeaderIfNotEmpty(header, c.options.DisplayNameHeader, subject.DisplayName)
	setHeaderIfNotEmpty(header, c.options.EmailHeader, subject.Email)
	if subject.EmailVerified {
		header.Set(c.options.EmailVerifiedHeader, strconv.FormatBool(subject.EmailVerified))
	}
	addHeaderValues(header, c.options.GroupsHeader, subject.Groups)
}

func setHeaderIfNotEmpty(header http.Header, name, value string) {
	if value != "" {
		header.Set(name, value)
	}
}

func addHeaderValues(header http.Header, name string, values []string) {
	for _, value := range values {
		header.Add(name, value)
	}
}

// EncodeFromContext forwards the authentication installed by
// NewAuthenticationFilter.
func (c *requestHeaderCodec) EncodeFromContext(ctx context.Context, req *http.Request) {
	c.Encode(req, AuthenticationFromContext(ctx))
}

// Decode reconstructs authentication without verifying its source.
func (c *requestHeaderCodec) Decode(req *http.Request) (*Authentication, error) {
	if !c.Has(req.Header) {
		return nil, ErrNotProvided
	}
	subject, _, err := c.decodeSubject(req.Header, false)
	if err != nil {
		return nil, err
	}
	actor, hasActor, err := c.decodeSubject(req.Header, true)
	if err != nil {
		return nil, err
	}
	info := &Authentication{Subject: subject}
	if hasActor {
		info.Actor = &actor
	}
	access, hasAccess, err := singleHeader(req.Header, c.options.AccessHeader)
	if err != nil {
		return nil, err
	}
	if hasAccess && access != "oauth2" {
		return nil, fmt.Errorf("request header codec: %s must be oauth2", c.options.AccessHeader)
	}
	audiences, _ := headerValues(req.Header, c.options.AudiencesHeader)
	scopes, _ := headerValues(req.Header, c.options.ScopesHeader)
	if hasAccess {
		info.Token = &authn.TokenInfo{Audiences: audiences, Scopes: scopes}
	}
	return info, nil
}

func (c *requestHeaderCodec) decodeSubject(header http.Header, actor bool) (Subject, bool, error) {
	options := c.options
	typeHeader := options.SubjectTypeHeader
	idHeader := options.UserIDHeader
	nameHeader := options.NameHeader
	displayNameHeader := options.DisplayNameHeader
	emailHeader := options.EmailHeader
	emailVerifiedHeader := options.EmailVerifiedHeader
	groupsHeader := options.GroupsHeader
	if actor {
		typeHeader = options.ActorTypeHeader
		idHeader = options.ActorHeader
		nameHeader = options.ActorNameHeader
		displayNameHeader = options.ActorDisplayNameHeader
		emailHeader = options.ActorEmailHeader
		emailVerifiedHeader = options.ActorEmailVerifiedHeader
		groupsHeader = options.ActorGroupsHeader
	}
	subjectType, hasType, err := singleHeader(header, typeHeader)
	if err != nil {
		return Subject{}, false, err
	}
	id, hasID, err := singleHeader(header, idHeader)
	if err != nil {
		return Subject{}, false, err
	}
	name, hasName, err := singleHeader(header, nameHeader)
	if err != nil {
		return Subject{}, false, err
	}
	displayName, hasDisplayName, err := singleHeader(header, displayNameHeader)
	if err != nil {
		return Subject{}, false, err
	}
	email, hasEmail, err := singleHeader(header, emailHeader)
	if err != nil {
		return Subject{}, false, err
	}
	emailVerifiedValue, hasEmailVerified, err := singleHeader(header, emailVerifiedHeader)
	if err != nil {
		return Subject{}, false, err
	}
	emailVerified := false
	if hasEmailVerified {
		emailVerified, err = strconv.ParseBool(emailVerifiedValue)
		if err != nil {
			return Subject{}, false, fmt.Errorf("request header codec: decode %s: %w", emailVerifiedHeader, err)
		}
	}
	groups, hasGroups := headerValues(header, groupsHeader)
	return Subject{
		Type:          authn.SubjectType(subjectType),
		ID:            id,
		Name:          name,
		DisplayName:   displayName,
		Email:         email,
		EmailVerified: emailVerified,
		Groups:        groups,
	}, hasType || hasID || hasName || hasDisplayName || hasEmail || hasEmailVerified || hasGroups, nil
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
	values := header.Values(name)
	return values, values != nil
}

// Has reports whether header contains any authentication field managed by the
// codec.
func (c *requestHeaderCodec) Has(header http.Header) bool {
	for _, name := range c.headerNames() {
		if _, present := headerValues(header, name); present {
			return true
		}
	}
	return false
}

// Clear removes every authentication header managed by the codec.
func (c *requestHeaderCodec) Clear(header http.Header) {
	for _, name := range c.headerNames() {
		header.Del(name)
	}
}

func (c *requestHeaderCodec) headerNames() []string {
	options := c.options
	return []string{
		options.SubjectTypeHeader,
		options.NameHeader,
		options.UserIDHeader,
		options.DisplayNameHeader,
		options.EmailHeader,
		options.EmailVerifiedHeader,
		options.GroupsHeader,
		options.ActorTypeHeader,
		options.ActorHeader,
		options.ActorNameHeader,
		options.ActorDisplayNameHeader,
		options.ActorEmailHeader,
		options.ActorEmailVerifiedHeader,
		options.ActorGroupsHeader,
		options.AccessHeader,
		options.AudiencesHeader,
		options.ScopesHeader,
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

// CIDRRequestHeaderTrustVerifier trusts direct peers matching an exact IP,
// CIDR, or the wildcard "*".
type CIDRRequestHeaderTrustVerifier struct {
	AllowedCIDRs []string
}

var _ RequestHeaderTrustVerifier = CIDRRequestHeaderTrustVerifier{}

func (v CIDRRequestHeaderTrustVerifier) VerifyRequest(req *http.Request) error {
	if RequestSourceIPInCIDR(v.AllowedCIDRs, req) {
		return nil
	}
	return fmt.Errorf("request header source IP is not trusted")
}

// TLSRequestHeaderTrustVerifier trusts client certificates whose chain was
// verified by the HTTP server and whose leaf Common Name is allowed.
type TLSRequestHeaderTrustVerifier struct {
	AllowedNames []string
}

var _ RequestHeaderTrustVerifier = TLSRequestHeaderTrustVerifier{}

func (v TLSRequestHeaderTrustVerifier) VerifyRequest(req *http.Request) error {
	if req.TLS == nil || len(req.TLS.VerifiedChains) == 0 || len(req.TLS.VerifiedChains[0]) == 0 {
		return fmt.Errorf("request header source TLS client certificate is not verified")
	}
	commonName := req.TLS.VerifiedChains[0][0].Subject.CommonName
	for _, allowed := range v.AllowedNames {
		if allowed == "*" || allowed == commonName {
			return nil
		}
	}
	return fmt.Errorf("request header source TLS client certificate is not trusted")
}

// RequestHeaderAuthenticator authenticates assertions from a trusted proxy.
// The proxy must remove client-supplied assertion headers.
type RequestHeaderAuthenticator struct {
	codec    RequestHeaderCodec
	verifier RequestHeaderTrustVerifier
}

var _ HTTPAuthenticator = (*RequestHeaderAuthenticator)(nil)

func NewRequestHeaderAuthenticator(codec RequestHeaderCodec, verifier RequestHeaderTrustVerifier) (*RequestHeaderAuthenticator, error) {
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

func (a *RequestHeaderAuthenticator) AuthenticateHTTP(_ http.ResponseWriter, req *http.Request) (*Authentication, error) {
	if !a.codec.Has(req.Header) {
		return nil, ErrNotProvided
	}
	if err := a.verifier.VerifyRequest(req); err != nil {
		return nil, fmt.Errorf("request header authenticator: verify trusted proxy: %w", err)
	}
	return a.codec.Decode(req)
}

// AuthenticationProxyRoundTripper clears request-header authentication
// assertions and optionally replaces them from one authentication source.
type AuthenticationProxyRoundTripper struct {
	Codec RequestHeaderCodec
	// Authentication resolves the assertion for one request. Nil clears
	// configured assertion headers without injecting an identity.
	Authentication func(context.Context) Authentication
	Transport      http.RoundTripper
}

var _ http.RoundTripper = (*AuthenticationProxyRoundTripper)(nil)
var _ httpclient.RequestAuthenticator = (*AuthenticationProxyRoundTripper)(nil)
var _ httpclient.WrappedRoundTripper = (*AuthenticationProxyRoundTripper)(nil)

// NewAuthenticationProxyRoundTripper forwards fixed authentication information.
func NewAuthenticationProxyRoundTripper(codec RequestHeaderCodec, info Authentication, transport http.RoundTripper) *AuthenticationProxyRoundTripper {
	snapshot := copyAuthentication(info)
	return &AuthenticationProxyRoundTripper{
		Codec:          codec,
		Authentication: func(context.Context) Authentication { return snapshot },
		Transport:      transport,
	}
}

// NewContextAuthenticationProxyRoundTripper forwards authentication stored in
// each request context by NewAuthenticationFilter.
func NewContextAuthenticationProxyRoundTripper(codec RequestHeaderCodec, transport http.RoundTripper) *AuthenticationProxyRoundTripper {
	return &AuthenticationProxyRoundTripper{Codec: codec, Authentication: AuthenticationFromContext, Transport: transport}
}

// NewRequestHeaderSanitizingRoundTripper removes configured authentication
// headers without injecting an identity.
func NewRequestHeaderSanitizingRoundTripper(codec RequestHeaderCodec, transport http.RoundTripper) *AuthenticationProxyRoundTripper {
	return &AuthenticationProxyRoundTripper{Codec: codec, Transport: transport}
}

// AuthenticateRequest replaces configured request-header assertions without
// sending the request.
func (rt *AuthenticationProxyRoundTripper) AuthenticateRequest(req *http.Request) error {
	if rt.Authentication != nil {
		rt.Codec.Encode(req, rt.Authentication(req.Context()))
	} else {
		rt.Codec.Clear(req.Header)
	}
	return nil
}

func (rt *AuthenticationProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := httpclient.CloneRequest(req)
	if err := rt.AuthenticateRequest(clone); err != nil {
		return nil, err
	}
	return rt.Transport.RoundTrip(clone)
}

func (rt *AuthenticationProxyRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.Transport
}
