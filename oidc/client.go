package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	ErrInvalidAccessToken  = errors.New("oidc: invalid access token")
	ErrStateMismatch       = errors.New("oidc: state mismatch")
	ErrUnsupportedEndpoint = errors.New("oidc: endpoint is not supported by the provider")
)

// EndpointError describes an OAuth 2.0 error response.
type EndpointError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	URI         string `json:"error_uri"`
	StatusCode  int    `json:"-"`
}

// Error returns the OAuth error code and description.
func (e *EndpointError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("oidc: endpoint returned %s: %s", e.Code, e.Description)
	}
	if e.Code != "" {
		return fmt.Sprintf("oidc: endpoint returned %s", e.Code)
	}
	return fmt.Sprintf("oidc: endpoint returned HTTP %d", e.StatusCode)
}

// ClientOptions configures one OpenID Connect client.
type ClientOptions struct {
	Issuer         string
	HTTPClient     *http.Client
	Authentication ClientAuthentication
	RedirectURL    string
	// Scopes configures Authorization Code and Refresh Token operations.
	// Client Credentials scopes belong to one ClientCredentialsTokenSource.
	Scopes                      []string
	IDTokenAudiences            []string
	AccessTokenValidation       AccessTokenValidation
	IntrospectionAuthentication *ClientAuthentication
	RevocationAuthentication    *ClientAuthentication
	// DiscoveryRefreshInterval controls automatic Provider Configuration
	// refresh. Zero and negative values disable automatic refresh.
	DiscoveryRefreshInterval time.Duration
}

// ClientCredentialsOptions binds one Client Credentials token source to one
// Resource Server and one exact scope set. An empty Resource lets the
// Authorization Server select its configured default resource.
type ClientCredentialsOptions struct {
	Resource string
	Scopes   []string
}

// ClientCredentialsTokenSource obtains and caches Client Credentials tokens
// for one immutable resource and scope profile. Sources created by the same
// Client share Provider Discovery but never share access tokens.
type ClientCredentialsTokenSource struct {
	client      *Client
	options     ClientCredentialsOptions
	tokenFlight SingleFlight[*oauth2.Token]
}

// DefaultDiscoveryRefreshInterval is the interval after which an operation
// starts refreshing a previously discovered provider configuration.
const DefaultDiscoveryRefreshInterval = 12 * time.Hour

// NewDefaultClientOptions returns client options with automatic Provider
// Configuration refresh enabled at DefaultDiscoveryRefreshInterval.
func NewDefaultClientOptions() ClientOptions {
	return ClientOptions{DiscoveryRefreshInterval: DefaultDiscoveryRefreshInterval}
}

// NewClientCredentialsTokenSource creates one target-bound token source.
func (c *Client) NewClientCredentialsTokenSource(options ClientCredentialsOptions) *ClientCredentialsTokenSource {
	options.Scopes = slices.Clone(options.Scopes)
	return &ClientCredentialsTokenSource{client: c, options: options}
}

// Token obtains and caches a Client Credentials token. Concurrent calls made
// while a token is unavailable share one token endpoint request.
func (s *ClientCredentialsTokenSource) Token(ctx context.Context) (*TokenSet, error) {
	token, err := s.tokenFlight.Get(ctx, clientCredentialsTokenState, s.getToken)
	if err != nil {
		return nil, err
	}
	return TokenSetFromOAuth2(token)
}

func clientCredentialsTokenState(token *oauth2.Token) CacheState {
	if token.Valid() {
		return CacheFresh
	}
	return CacheExpired
}

func (s *ClientCredentialsTokenSource) getToken(ctx context.Context) (*oauth2.Token, error) {
	configuration, err := s.client.getClientConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	clientCredentials, err := configuration.GetClientCredentialsConfiguration(s.options)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, configuration.HTTPClient)
	token, err := clientCredentials.Token(ctx)
	if err != nil {
		return nil, EndpointTokenError(err)
	}
	return token, nil
}

// RefreshTokens exchanges the current refresh token for replacement tokens.
// If the response omits a refresh token or ID Token, the current value is
// retained. A returned ID Token must identify the same subject and audience.
func (c *Client) RefreshTokens(ctx context.Context, current *TokenSet) (*TokenSet, error) {
	// oauth2.Config.TokenSource owns the RFC 6749 refresh request, client
	// authentication, response parsing, and refresh-token rotation behavior.
	// In particular, it retains the submitted refresh token when the response
	// omits one and replaces it when the response returns one; do not merge that
	// field again here.
	// Supplying only the refresh token makes the oauth2 token invalid by its
	// normal rules, so this explicit operation enters that refresh path without
	// inventing an access-token expiry time.
	refresh := &oauth2.Token{
		RefreshToken: current.RefreshToken,
	}
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	refreshConfiguration, err := configuration.GetRefreshTokenConfiguration()
	if err != nil {
		return nil, err
	}
	verifier, err := configuration.GetIDTokenVerifier()
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, configuration.HTTPClient)
	token, err := refreshConfiguration.TokenSource(ctx, refresh).Token()
	if err != nil {
		return nil, EndpointTokenError(err)
	}
	set, err := TokenSetFromOAuth2(token)
	if err != nil {
		return nil, err
	}
	// RFC 6749 section 6 makes scope optional when it is unchanged by the
	// refresh response, so TokenSet must retain the effective scope here.
	if len(set.Scopes) == 0 {
		set.Scopes = current.Scopes
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		// OpenID Connect Core section 12.2 allows a refresh response to omit
		// id_token. In that case the previously verified identity remains the
		// effective ID Token associated with this TokenSet.
		set.IDToken = current.IDToken
		return set, nil
	}
	idToken, err := verifier.Verify(ctx, rawIDToken, IDTokenChecks{
		AccessToken: set.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	// OpenID Connect Core section 12.2 requires a refreshed ID Token to keep
	// the original subject and audience. Issuer continuity was already checked
	// by VerifyIDToken against the provider discovered by this Client.
	if idToken.Subject != current.IDToken.Subject {
		return nil, fmt.Errorf("oidc: refreshed ID Token subject changed")
	}
	if len(idToken.Audience) != len(current.IDToken.Audience) {
		return nil, fmt.Errorf("oidc: refreshed ID Token audience changed")
	}
	for _, audience := range idToken.Audience {
		if !slices.Contains(current.IDToken.Audience, audience) {
			return nil, fmt.Errorf("oidc: refreshed ID Token audience changed")
		}
	}
	// Section 12.2 also requires auth_time, when present, to continue to
	// represent the original authentication time. It can only be compared when
	// both ID Tokens contain the claim.
	if idToken.AuthTime != nil && current.IDToken.AuthTime != nil && !idToken.AuthTime.Time().Equal(current.IDToken.AuthTime.Time()) {
		return nil, fmt.Errorf("oidc: refreshed ID Token authentication time changed")
	}
	// A refreshed ID Token should omit nonce, but section 12.2 requires it to
	// equal the original nonce when the provider includes it.
	if idToken.Nonce != "" && idToken.Nonce != current.IDToken.Nonce {
		return nil, fmt.Errorf("oidc: refreshed ID Token nonce changed")
	}
	set.IDToken = idToken
	return set, nil
}

// TokenSetFromOAuth2 converts an OAuth 2.0 token into this package's public
// token model. It does not verify or populate an OpenID Connect ID Token.
func TokenSetFromOAuth2(token *oauth2.Token) (*TokenSet, error) {
	if token.TokenType == "" {
		return nil, fmt.Errorf("oidc: token response is missing token_type")
	}
	set := &TokenSet{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}
	if scopes, ok := token.Extra("scope").(string); ok {
		set.Scopes = strings.Fields(scopes)
	}
	return set, nil
}

// EndpointTokenError converts an oauth2.RetrieveError into EndpointError.
// Errors produced outside an OAuth 2.0 token endpoint are returned unchanged.
func EndpointTokenError(err error) error {
	var retrieval *oauth2.RetrieveError
	if !errors.As(err, &retrieval) {
		return err
	}
	return &EndpointError{
		Code:        retrieval.ErrorCode,
		Description: retrieval.ErrorDescription,
		URI:         retrieval.ErrorURI,
		StatusCode:  retrieval.Response.StatusCode,
	}
}

// DecodeEndpointResponse closes response.Body, decodes a successful JSON
// response into target, and converts unsuccessful responses to EndpointError.
func DecodeEndpointResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		endpoint := &EndpointError{
			StatusCode: response.StatusCode,
		}
		_ = json.NewDecoder(response.Body).Decode(endpoint)
		return endpoint
	}
	return json.NewDecoder(response.Body).Decode(target)
}

// DecodeEndpointResponseStatus closes response.Body and decodes target only
// when the response has the expected status. Every other status is returned as
// EndpointError, including an unexpected successful status.
func DecodeEndpointResponseStatus(response *http.Response, target any, expectedStatus int) error {
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		endpoint := &EndpointError{StatusCode: response.StatusCode}
		_ = json.NewDecoder(response.Body).Decode(endpoint)
		return endpoint
	}
	if expectedStatus == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}
