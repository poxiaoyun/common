package oidc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// ClientConfiguration is one immutable combination of ClientOptions and a
// Provider Metadata snapshot. Client owns its lifetime and never returns it to
// callers; exported fields allow the package's operation modules to consume
// one consistent version without rebuilding derived state. Derived
// capabilities are initialized only when an operation needs them.
type ClientConfiguration struct {
	Metadata                  OpenIDProviderMetadata
	HTTPClient                *http.Client
	Authentication            ClientAuthentication
	KeySet                    *KeySet
	ProviderMetadataRefreshAt time.Time

	options                   ClientOptions
	accessTokenVerifierFlight SingleFlight[AccessTokenVerifier]
}

// Client combines outbound OpenID Connect and OAuth 2.0 operations with
// explicitly configured inbound token validation. Provider metadata is
// discovered on first use and combined with the options into one immutable
// ClientConfiguration snapshot.
type Client struct {
	options             ClientOptions
	configurationFlight SingleFlight[*ClientConfiguration]
}

// NewClient constructs a client without performing Discovery. The first
// operation that needs provider metadata performs the initial request.
func NewClient(options ClientOptions) (*Client, error) {
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}
	if err := ValidateClientAuthMethod(options.Authentication.Method); err != nil {
		return nil, err
	}
	if options.IntrospectionAuthentication != nil {
		if err := ValidateClientAuthMethod(options.IntrospectionAuthentication.Method); err != nil {
			return nil, err
		}
	}
	if options.RevocationAuthentication != nil {
		if err := ValidateClientAuthMethod(options.RevocationAuthentication.Method); err != nil {
			return nil, err
		}
	}
	switch options.AccessTokenValidation.Mode {
	case "":
	case AccessTokenValidationJWT, AccessTokenValidationIntrospection, AccessTokenValidationAuto:
		if options.AccessTokenValidation.Audience == "" {
			return nil, fmt.Errorf("oidc: access-token audience is required")
		}
	default:
		return nil, fmt.Errorf("oidc: unsupported access-token validation mode %q", options.AccessTokenValidation.Mode)
	}
	return &Client{options: options}, nil
}

// getClientConfiguration deliberately remains private because a configuration
// contains mutable protocol caches and must never escape its owning Client.
func (c *Client) getClientConfiguration(ctx context.Context) (*ClientConfiguration, error) {
	return c.configurationFlight.Get(ctx, c.clientConfigurationState, c.loadClientConfiguration)
}

func (c *Client) clientConfigurationState(current *ClientConfiguration) CacheState {
	if c.options.DiscoveryRefreshInterval > 0 && !time.Now().Before(current.ProviderMetadataRefreshAt) {
		return CacheStale
	}
	return CacheFresh
}

func (c *Client) loadClientConfiguration(ctx context.Context) (*ClientConfiguration, error) {
	metadata, err := DiscoverProviderMetadata(ctx, c.options.Issuer, c.options.HTTPClient)
	if err != nil {
		return nil, err
	}
	current := newClientConfiguration(c.options, *metadata)
	if c.options.DiscoveryRefreshInterval > 0 {
		current.ProviderMetadataRefreshAt = time.Now().Add(c.options.DiscoveryRefreshInterval)
	}
	return current, nil
}

func newClientConfiguration(options ClientOptions, metadata OpenIDProviderMetadata) *ClientConfiguration {
	keys := &KeySet{URL: metadata.JWKSURI, Client: options.HTTPClient}
	return &ClientConfiguration{
		Metadata:       metadata,
		HTTPClient:     options.HTTPClient,
		Authentication: options.Authentication,
		KeySet:         keys,
		options:        options,
	}
}

// GetAccessTokenVerifier returns the reusable access-token verifier. Concurrent
// first calls share one construction.
func (c *ClientConfiguration) GetAccessTokenVerifier(ctx context.Context) (AccessTokenVerifier, error) {
	return c.accessTokenVerifierFlight.Get(ctx, accessTokenVerifierState, c.newAccessTokenVerifier)
}

func accessTokenVerifierState(AccessTokenVerifier) CacheState {
	return CacheFresh
}

func (c *ClientConfiguration) newAccessTokenVerifier(context.Context) (AccessTokenVerifier, error) {
	return NewAccessTokenVerifier(c.options, c.Metadata, c.KeySet)
}

// GetIDTokenVerifier returns an ID Token verifier that shares this
// configuration's reusable KeySet.
func (c *ClientConfiguration) GetIDTokenVerifier() (*IDTokenVerifier, error) {
	return NewIDTokenVerifier(c.options, c.Metadata, c.KeySet)
}

// GetOAuth2TokenConfiguration returns the token endpoint configuration.
func (c *ClientConfiguration) GetOAuth2TokenConfiguration() (oauth2.Config, error) {
	return newOAuth2TokenConfiguration(c.options, c.Metadata)
}

// GetAuthorizationCodeConfiguration returns the authorization-code configuration.
func (c *ClientConfiguration) GetAuthorizationCodeConfiguration() (oauth2.Config, error) {
	token, err := c.GetOAuth2TokenConfiguration()
	if err != nil {
		return oauth2.Config{}, err
	}
	if _, err := c.GetIDTokenVerifier(); err != nil {
		return oauth2.Config{}, err
	}
	return newAuthorizationCodeConfiguration(token, c.Metadata)
}

// GetClientCredentialsConfiguration returns the client_credentials
// configuration for one target-bound token source.
func (c *ClientConfiguration) GetClientCredentialsConfiguration(options ClientCredentialsOptions) (clientcredentials.Config, error) {
	return newClientCredentialsConfiguration(c.options, options, c.Metadata)
}

// GetRefreshTokenConfiguration returns the reusable refresh-token configuration.
func (c *ClientConfiguration) GetRefreshTokenConfiguration() (oauth2.Config, error) {
	if !ProviderSupportsGrantType(c.Metadata, "refresh_token") {
		return oauth2.Config{}, fmt.Errorf("oidc: provider does not support the refresh_token grant type")
	}
	return c.GetOAuth2TokenConfiguration()
}

// GetRevocationAuthentication returns the revocation endpoint authentication.
func (c *ClientConfiguration) GetRevocationAuthentication() (ClientAuthentication, error) {
	return newRevocationAuthentication(c.options, c.Metadata)
}

// GetEndSessionEndpoint returns the parsed end-session endpoint.
func (c *ClientConfiguration) GetEndSessionEndpoint() (*url.URL, error) {
	if c.Metadata.EndSessionEndpoint == "" {
		return nil, fmt.Errorf("%w: end session", ErrUnsupportedEndpoint)
	}
	return url.Parse(c.Metadata.EndSessionEndpoint)
}

func newOAuth2TokenConfiguration(options ClientOptions, metadata OpenIDProviderMetadata) (oauth2.Config, error) {
	if metadata.TokenEndpoint == "" {
		return oauth2.Config{}, fmt.Errorf("%w: token", ErrUnsupportedEndpoint)
	}
	method, err := SelectClientAuthMethod(options.Authentication.Method, metadata.TokenEndpointAuthMethodsSupported)
	if err != nil {
		return oauth2.Config{}, err
	}
	secret := options.Authentication.ClientSecret
	if method == ClientAuthNone {
		secret = ""
	}
	scopes := options.Scopes
	if !slices.Contains(scopes, "openid") {
		scopes = append([]string{"openid"}, scopes...)
	}
	return oauth2.Config{
		ClientID:     options.Authentication.ClientID,
		ClientSecret: secret,
		RedirectURL:  options.RedirectURL,
		Scopes:       scopes,
		Endpoint:     oauth2.Endpoint{TokenURL: metadata.TokenEndpoint, AuthStyle: ClientAuthStyle(method)},
	}, nil
}

func newAuthorizationCodeConfiguration(
	token oauth2.Config,
	metadata OpenIDProviderMetadata,
) (oauth2.Config, error) {
	if metadata.AuthorizationEndpoint == "" {
		return oauth2.Config{}, fmt.Errorf("%w: authorization", ErrUnsupportedEndpoint)
	}
	if !ProviderSupportsGrantType(metadata, "authorization_code") {
		return oauth2.Config{}, fmt.Errorf("oidc: provider does not support the authorization_code grant type")
	}
	if !slices.Contains(metadata.ResponseTypesSupported, "code") {
		return oauth2.Config{}, fmt.Errorf("oidc: provider does not support the authorization code response type")
	}
	if len(metadata.CodeChallengeMethodsSupported) > 0 && !slices.Contains(metadata.CodeChallengeMethodsSupported, "S256") {
		return oauth2.Config{}, fmt.Errorf("oidc: provider does not support S256 PKCE")
	}
	token.Endpoint.AuthURL = metadata.AuthorizationEndpoint
	return token, nil
}

func newClientCredentialsConfiguration(client ClientOptions, options ClientCredentialsOptions, metadata OpenIDProviderMetadata) (clientcredentials.Config, error) {
	if metadata.TokenEndpoint == "" {
		return clientcredentials.Config{}, fmt.Errorf("%w: token", ErrUnsupportedEndpoint)
	}
	if !ProviderSupportsGrantType(metadata, "client_credentials") {
		return clientcredentials.Config{}, fmt.Errorf("oidc: provider does not support the client_credentials grant type")
	}
	method, err := SelectClientAuthMethod(client.Authentication.Method, metadata.TokenEndpointAuthMethodsSupported)
	if err != nil {
		return clientcredentials.Config{}, err
	}
	secret := client.Authentication.ClientSecret
	if method == ClientAuthNone {
		secret = ""
	}
	current := clientcredentials.Config{
		ClientID:     client.Authentication.ClientID,
		ClientSecret: secret,
		TokenURL:     metadata.TokenEndpoint,
		Scopes:       options.Scopes,
		AuthStyle:    ClientAuthStyle(method),
	}
	if options.Resource != "" {
		current.EndpointParams = url.Values{"resource": {options.Resource}}
	}
	return current, nil
}

func newRevocationAuthentication(options ClientOptions, metadata OpenIDProviderMetadata) (ClientAuthentication, error) {
	if metadata.RevocationEndpoint == "" {
		return ClientAuthentication{}, fmt.Errorf("%w: revocation", ErrUnsupportedEndpoint)
	}
	current := options.Authentication
	if options.RevocationAuthentication != nil {
		current = *options.RevocationAuthentication
	}
	method, err := SelectClientAuthMethod(current.Method, metadata.RevocationEndpointAuthMethodsSupported)
	if err != nil {
		return ClientAuthentication{}, fmt.Errorf("oidc: revocation endpoint: %w", err)
	}
	current.Method = method
	return current, nil
}

// DiscoverProviderMetadata obtains and validates an OpenID Provider Metadata document.
func DiscoverProviderMetadata(ctx context.Context, issuer string, httpClient *http.Client) (*OpenIDProviderMetadata, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	discoveryURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	metadata := &OpenIDProviderMetadata{}
	if err := DecodeEndpointResponse(response, metadata); err != nil {
		return nil, err
	}
	if metadata.Issuer != issuer {
		return nil, fmt.Errorf("oidc: issuer mismatch: expected %q, got %q", issuer, metadata.Issuer)
	}
	return metadata, nil
}

// ProviderSupportsGrantType reports support according to the discovered
// grant_types_supported value and the RFC 8414 defaults when it is omitted.
func ProviderSupportsGrantType(metadata OpenIDProviderMetadata, grantType string) bool {
	if metadata.GrantTypesSupported == nil {
		return grantType == "authorization_code" || grantType == "implicit"
	}
	return slices.Contains(metadata.GrantTypesSupported, grantType)
}

// ValidateClientAuthMethod reports whether method is supported by this client.
func ValidateClientAuthMethod(method ClientAuthMethod) error {
	switch method {
	case "", ClientAuthNone, ClientAuthSecretBasic, ClientAuthSecretPost:
		return nil
	default:
		return fmt.Errorf("oidc: unsupported client authentication method %q", method)
	}
}

// ClientAuthStyle converts a client authentication method to oauth2.AuthStyle.
func ClientAuthStyle(method ClientAuthMethod) oauth2.AuthStyle {
	if method == ClientAuthSecretBasic {
		return oauth2.AuthStyleInHeader
	}
	return oauth2.AuthStyleInParams
}

// SelectClientAuthMethod chooses the configured or advertised client
// authentication method. An omitted advertised list defaults to
// client_secret_basic.
func SelectClientAuthMethod(configured ClientAuthMethod, supported []string) (ClientAuthMethod, error) {
	if supported == nil {
		supported = []string{string(ClientAuthSecretBasic)}
	}
	if configured != "" {
		if slices.Contains(supported, string(configured)) {
			return configured, nil
		}
		return "", fmt.Errorf("oidc: client authentication method %q is not supported", configured)
	}
	if slices.Contains(supported, string(ClientAuthSecretBasic)) {
		return ClientAuthSecretBasic, nil
	}
	if slices.Contains(supported, string(ClientAuthSecretPost)) {
		return ClientAuthSecretPost, nil
	}
	if slices.Contains(supported, string(ClientAuthNone)) {
		return ClientAuthNone, nil
	}
	return "", fmt.Errorf("oidc: provider does not advertise a supported client authentication method")
}
