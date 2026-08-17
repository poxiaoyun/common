package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// AccessTokenVerifier validates an access token and returns its verified
// claims.
type AccessTokenVerifier interface {
	// Verify validates raw and returns its claims.
	Verify(ctx context.Context, raw string) (*AccessToken, error)
}

// JWTAccessTokenVerifier validates RFC 9068 JWT access tokens locally.
type JWTAccessTokenVerifier struct {
	issuer     string
	audience   string
	algorithms []string
	keys       *KeySet
}

// IntrospectionAccessTokenVerifier validates access tokens through RFC 7662
// token introspection.
type IntrospectionAccessTokenVerifier struct {
	issuer         string
	audience       string
	endpoint       string
	authentication ClientAuthentication
	httpClient     *http.Client
}

// AutoAccessTokenVerifier routes JWT-shaped tokens to JWT and all other
// tokens to Introspection. It never retries with the other verifier.
type AutoAccessTokenVerifier struct {
	jwt           AccessTokenVerifier
	introspection AccessTokenVerifier
}

// JWTAccessTokenClaims contains the claims required or defined by the RFC
// 9068 JWT Profile for OAuth 2.0 Access Tokens.
type JWTAccessTokenClaims struct {
	JWTClaims
	ClientID string       `json:"client_id,omitempty"`
	Scope    string       `json:"scope,omitempty"`
	Actor    *ActorClaims `json:"act,omitempty"`
}

type tokenIntrospectionResponse struct {
	JWTClaims
	Active   bool         `json:"active"`
	ClientID string       `json:"client_id,omitempty"`
	Username string       `json:"username,omitempty"`
	Scope    string       `json:"scope,omitempty"`
	Actor    *ActorClaims `json:"act,omitempty"`
}

var (
	_ AccessTokenVerifier = &JWTAccessTokenVerifier{}
	_ AccessTokenVerifier = &IntrospectionAccessTokenVerifier{}
	_ AccessTokenVerifier = &AutoAccessTokenVerifier{}
)

// VerifyAccessToken validates a token using the strategy selected at Client
// construction.
func (c *Client) VerifyAccessToken(ctx context.Context, raw string) (*AccessToken, error) {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	verifier, err := configuration.GetAccessTokenVerifier(ctx)
	if err != nil {
		return nil, err
	}
	return verifier.Verify(ctx, raw)
}

// NewAccessTokenVerifier constructs the access-token verifier selected by the
// client options from one provider metadata snapshot and shared KeySet.
func NewAccessTokenVerifier(options ClientOptions, metadata OpenIDProviderMetadata, keys *KeySet) (AccessTokenVerifier, error) {
	switch options.AccessTokenValidation.Mode {
	case AccessTokenValidationJWT:
		return NewJWTAccessTokenVerifier(options, metadata, keys)
	case AccessTokenValidationIntrospection:
		return NewIntrospectionAccessTokenVerifier(options, metadata)
	case "", AccessTokenValidationAuto:
		jwtVerifier, err := NewJWTAccessTokenVerifier(options, metadata, keys)
		if err != nil {
			return nil, err
		}
		introspectionVerifier, err := NewIntrospectionAccessTokenVerifier(options, metadata)
		if err != nil {
			return nil, err
		}
		return NewAutoAccessTokenVerifier(jwtVerifier, introspectionVerifier), nil
	default:
		return nil, fmt.Errorf("oidc: unsupported access-token validation mode %q", options.AccessTokenValidation.Mode)
	}
}

// NewJWTAccessTokenVerifier constructs an RFC 9068 JWT access-token verifier
// using the supplied reusable KeySet.
func NewJWTAccessTokenVerifier(options ClientOptions, metadata OpenIDProviderMetadata, keys *KeySet) (*JWTAccessTokenVerifier, error) {
	if options.AccessTokenValidation.Audience == "" {
		return nil, fmt.Errorf("oidc: access-token audience is required")
	}
	if metadata.JWKSURI == "" {
		return nil, fmt.Errorf("%w: JWKS", ErrUnsupportedEndpoint)
	}
	algorithms := options.AccessTokenValidation.SigningAlgorithms
	if len(algorithms) == 0 {
		algorithms = DefaultJWTAccessTokenSigningAlgorithms()
	}
	return &JWTAccessTokenVerifier{
		issuer:     metadata.Issuer,
		audience:   options.AccessTokenValidation.Audience,
		algorithms: algorithms,
		keys:       keys,
	}, nil
}

// NewIntrospectionAccessTokenVerifier constructs an RFC 7662 token
// introspection verifier.
func NewIntrospectionAccessTokenVerifier(options ClientOptions, metadata OpenIDProviderMetadata) (*IntrospectionAccessTokenVerifier, error) {
	if options.AccessTokenValidation.Audience == "" {
		return nil, fmt.Errorf("oidc: access-token audience is required")
	}
	if metadata.IntrospectionEndpoint == "" {
		return nil, fmt.Errorf("%w: introspection", ErrUnsupportedEndpoint)
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authentication := ClientAuthentication{
		ClientID:     options.Authentication.ClientID,
		ClientSecret: options.Authentication.ClientSecret,
		Method:       options.Authentication.Method,
	}
	if options.IntrospectionAuthentication != nil {
		authentication = *options.IntrospectionAuthentication
	}
	method, err := SelectClientAuthMethod(authentication.Method, metadata.IntrospectionEndpointAuthMethodsSupported)
	if err != nil {
		return nil, fmt.Errorf("oidc: introspection endpoint: %w", err)
	}
	authentication.Method = method
	return &IntrospectionAccessTokenVerifier{
		issuer:         metadata.Issuer,
		audience:       options.AccessTokenValidation.Audience,
		endpoint:       metadata.IntrospectionEndpoint,
		authentication: authentication,
		httpClient:     httpClient,
	}, nil
}

// NewAutoAccessTokenVerifier composes JWT and introspection verifiers using
// token-shape routing without validation fallback.
func NewAutoAccessTokenVerifier(jwtVerifier, introspectionVerifier AccessTokenVerifier) *AutoAccessTokenVerifier {
	return &AutoAccessTokenVerifier{jwt: jwtVerifier, introspection: introspectionVerifier}
}

// Verify routes raw to exactly one verifier based on its serialized shape.
func (v *AutoAccessTokenVerifier) Verify(ctx context.Context, raw string) (*AccessToken, error) {
	if hasJWTHeader(raw) {
		return v.jwt.Verify(ctx, raw)
	}
	return v.introspection.Verify(ctx, raw)
}

// Verify validates an RFC 9068 JWT access token.
func (v *JWTAccessTokenVerifier) Verify(ctx context.Context, raw string) (*AccessToken, error) {
	payload, header, err := v.keys.Verify(ctx, raw, v.algorithms)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAccessToken, err)
	}
	typeValue := header.Type
	if !strings.EqualFold(typeValue, "at+jwt") && !strings.EqualFold(typeValue, "application/at+jwt") {
		return nil, fmt.Errorf("%w: JWT type is not at+jwt", ErrInvalidAccessToken)
	}
	claims := JWTAccessTokenClaims{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAccessToken, err)
	}
	if claims.Issuer == "" || claims.Subject == "" || len(claims.Audience) == 0 || claims.Expiry == nil || claims.IssuedAt == nil || claims.ID == "" || claims.ClientID == "" {
		return nil, fmt.Errorf("%w: JWT is missing required claims", ErrInvalidAccessToken)
	}
	if claims.Actor != nil && claims.Actor.Subject == "" {
		return nil, fmt.Errorf("%w: JWT actor is missing sub", ErrInvalidAccessToken)
	}
	if claims.Issuer != v.issuer {
		return nil, fmt.Errorf("%w: JWT issuer mismatch", ErrInvalidAccessToken)
	}
	if !slices.Contains(claims.Audience, v.audience) {
		return nil, fmt.Errorf("%w: JWT audience mismatch", ErrInvalidAccessToken)
	}
	now := time.Now()
	if !claims.Expiry.Time().After(now) {
		return nil, fmt.Errorf("%w: JWT is expired", ErrInvalidAccessToken)
	}
	if claims.NotBefore != nil && now.Before(claims.NotBefore.Time()) {
		return nil, fmt.Errorf("%w: JWT is not valid yet", ErrInvalidAccessToken)
	}
	if now.Before(claims.IssuedAt.Time()) {
		return nil, fmt.Errorf("%w: JWT was issued in the future", ErrInvalidAccessToken)
	}
	return &AccessToken{
		Issuer:   claims.Issuer,
		Subject:  claims.Subject,
		Audience: []string(claims.Audience),
		Expiry:   claims.Expiry.Time(),
		IssuedAt: claims.IssuedAt.Time(),
		ID:       claims.ID,
		ClientID: claims.ClientID,
		Scopes:   strings.Fields(claims.Scope),
		Actor:    claims.Actor,
		claims:   payload,
	}, nil
}

// Verify introspects an access token and validates the returned metadata.
func (v *IntrospectionAccessTokenVerifier) Verify(ctx context.Context, raw string) (*AccessToken, error) {
	form := url.Values{
		"token": {raw},
	}
	if v.authentication.Method == ClientAuthSecretPost {
		form.Set("client_id", v.authentication.ClientID)
		form.Set("client_secret", v.authentication.ClientSecret)
	}
	if v.authentication.Method == ClientAuthNone {
		form.Set("client_id", v.authentication.ClientID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if v.authentication.Method == ClientAuthSecretBasic {
		request.SetBasicAuth(url.QueryEscape(v.authentication.ClientID), url.QueryEscape(v.authentication.ClientSecret))
	}
	response, err := v.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	var payload json.RawMessage
	if err := DecodeEndpointResponse(response, &payload); err != nil {
		return nil, err
	}
	claims := tokenIntrospectionResponse{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	if !claims.Active {
		return nil, fmt.Errorf("%w: introspection response is inactive", ErrInvalidAccessToken)
	}
	if claims.Actor != nil && claims.Actor.Subject == "" {
		return nil, fmt.Errorf("%w: introspection actor is missing sub", ErrInvalidAccessToken)
	}
	if claims.Issuer != "" && claims.Issuer != v.issuer {
		return nil, fmt.Errorf("%w: introspection issuer mismatch", ErrInvalidAccessToken)
	}
	if !slices.Contains(claims.Audience, v.audience) {
		return nil, fmt.Errorf("%w: introspection audience mismatch", ErrInvalidAccessToken)
	}
	if claims.Expiry != nil && !claims.Expiry.Time().After(time.Now()) {
		return nil, fmt.Errorf("%w: introspection response is expired", ErrInvalidAccessToken)
	}
	if claims.NotBefore != nil && time.Now().Before(claims.NotBefore.Time()) {
		return nil, fmt.Errorf("%w: introspection response is not valid yet", ErrInvalidAccessToken)
	}
	token := &AccessToken{
		Issuer:   claims.Issuer,
		Subject:  claims.Subject,
		Audience: []string(claims.Audience),
		ID:       claims.ID,
		ClientID: claims.ClientID,
		Username: claims.Username,
		Scopes:   strings.Fields(claims.Scope),
		Actor:    claims.Actor,
		claims:   payload,
	}
	if claims.Expiry != nil {
		token.Expiry = claims.Expiry.Time()
	}
	if claims.IssuedAt != nil {
		token.IssuedAt = claims.IssuedAt.Time()
	}
	return token, nil
}

// hasJWTHeader identifies the Compact JWS shape used by JWT access tokens.
// RFC 7515 sections 3.1 and 4.1.1 define the three-part serialization and
// require the protected JOSE Header to contain a string alg parameter. RFC
// 9068 section 2.1 requires typ=at+jwt and a signed algorithm, but those are
// validation rules enforced by JWTAccessTokenVerifier, not routing rules. A
// JWT-shaped invalid token must stay on the JWT path instead of falling back
// to introspection.
func hasJWTHeader(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	protected := struct {
		Algorithm string `json:"alg"`
	}{}
	if err := json.Unmarshal(header, &protected); err != nil {
		return false
	}
	return protected.Algorithm != ""
}
