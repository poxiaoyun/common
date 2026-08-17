package oidc

import (
	"encoding/json"
	"strconv"
	"time"
)

// ClientAuthMethod identifies an OAuth 2.0 client authentication method.
type ClientAuthMethod string

const (
	ClientAuthNone        ClientAuthMethod = "none"
	ClientAuthSecretBasic ClientAuthMethod = "client_secret_basic"
	ClientAuthSecretPost  ClientAuthMethod = "client_secret_post"
)

// AccessTokenValidationMode selects an access-token validation strategy.
type AccessTokenValidationMode string

const (
	AccessTokenValidationJWT           AccessTokenValidationMode = "jwt"
	AccessTokenValidationIntrospection AccessTokenValidationMode = "introspection"
	AccessTokenValidationAuto          AccessTokenValidationMode = "auto"
)

// TokenTypeHint identifies the kind of token sent to the revocation endpoint.
type TokenTypeHint string

const (
	TokenTypeAccessToken  TokenTypeHint = "access_token"
	TokenTypeRefreshToken TokenTypeHint = "refresh_token"
)

// ClientAuthentication configures credentials for one OAuth 2.0 endpoint.
type ClientAuthentication struct {
	ClientID     string
	ClientSecret string
	Method       ClientAuthMethod
}

// AccessTokenValidation configures inbound access-token validation.
type AccessTokenValidation struct {
	// Mode selects the verifier. An empty value uses AccessTokenValidationAuto.
	Mode     AccessTokenValidationMode
	Audience string
	// SigningAlgorithms narrows the asymmetric JWT algorithms accepted from
	// JWKS. An empty value uses DefaultJWTAccessTokenSigningAlgorithms.
	SigningAlgorithms []string
}

// DefaultJWTAccessTokenSigningAlgorithms returns every asymmetric JWT
// signature algorithm supported by the client for JWKS verification.
func DefaultJWTAccessTokenSigningAlgorithms() []string {
	return []string{
		"RS256", "RS384", "RS512",
		"PS256", "PS384", "PS512",
		"ES256", "ES384", "ES512",
		"EdDSA",
	}
}

// OpenIDProviderMetadata is the OpenID Provider Metadata document used by the
// client. It includes the OAuth 2.0 endpoint extensions consumed by this
// package.
type OpenIDProviderMetadata struct {
	Issuer                                    string   `json:"issuer"`
	AuthorizationEndpoint                     string   `json:"authorization_endpoint"`
	TokenEndpoint                             string   `json:"token_endpoint,omitempty"`
	UserInfoEndpoint                          string   `json:"userinfo_endpoint,omitempty"`
	JWKSURI                                   string   `json:"jwks_uri"`
	ResponseTypesSupported                    []string `json:"response_types_supported"`
	SubjectTypesSupported                     []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported          []string `json:"id_token_signing_alg_values_supported"`
	UserInfoSigningAlgValuesSupported         []string `json:"userinfo_signing_alg_values_supported,omitempty"`
	TokenEndpointAuthMethodsSupported         []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	GrantTypesSupported                       []string `json:"grant_types_supported,omitempty"`
	RegistrationEndpoint                      string   `json:"registration_endpoint,omitempty"`
	RevocationEndpoint                        string   `json:"revocation_endpoint,omitempty"`
	RevocationEndpointAuthMethodsSupported    []string `json:"revocation_endpoint_auth_methods_supported,omitempty"`
	IntrospectionEndpoint                     string   `json:"introspection_endpoint,omitempty"`
	IntrospectionEndpointAuthMethodsSupported []string `json:"introspection_endpoint_auth_methods_supported,omitempty"`
	EndSessionEndpoint                        string   `json:"end_session_endpoint,omitempty"`
	CodeChallengeMethodsSupported             []string `json:"code_challenge_methods_supported,omitempty"`
}

// TokenSet contains the effective tokens obtained from the Token Endpoint.
// IDToken is populated only after an OpenID Connect ID Token is verified.
type TokenSet struct {
	AccessToken  string
	TokenType    string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
	IDToken      *IDToken
}

// IDToken contains a client-side projection of a verified OpenID Connect ID
// Token. DecodeClaims decodes the original claim names and values.
type IDToken struct {
	Raw string
	IDTokenClaims

	claims json.RawMessage
}

// IDTokenClaims contains the claims defined for ID Tokens by OpenID Connect
// Core. Providers may include additional claims, which callers can decode
// separately from IDToken.
type IDTokenClaims struct {
	JWTClaims
	Nonce                               string        `json:"nonce,omitempty"`
	AuthTime                            *UnixTime     `json:"auth_time,omitempty"`
	AuthenticationContextClassReference string        `json:"acr,omitempty"`
	AuthenticationMethodsReferences     []string      `json:"amr,omitempty"`
	AuthorizedParty                     string        `json:"azp,omitempty"`
	AccessTokenHash                     string        `json:"at_hash,omitempty"`
	AuthorizationCodeHash               string        `json:"c_hash,omitempty"`
	Name                                string        `json:"name,omitempty"`
	GivenName                           string        `json:"given_name,omitempty"`
	FamilyName                          string        `json:"family_name,omitempty"`
	MiddleName                          string        `json:"middle_name,omitempty"`
	Nickname                            string        `json:"nickname,omitempty"`
	PreferredUsername                   string        `json:"preferred_username,omitempty"`
	Profile                             string        `json:"profile,omitempty"`
	Picture                             string        `json:"picture,omitempty"`
	Website                             string        `json:"website,omitempty"`
	Email                               string        `json:"email,omitempty"`
	EmailVerified                       *bool         `json:"email_verified,omitempty"`
	Gender                              string        `json:"gender,omitempty"`
	Birthdate                           string        `json:"birthdate,omitempty"`
	ZoneInfo                            string        `json:"zoneinfo,omitempty"`
	Locale                              string        `json:"locale,omitempty"`
	PhoneNumber                         string        `json:"phone_number,omitempty"`
	PhoneNumberVerified                 *bool         `json:"phone_number_verified,omitempty"`
	Address                             *AddressClaim `json:"address,omitempty"`
	UpdatedAt                           *UnixTime     `json:"updated_at,omitempty"`
}

// JWTClaims contains the Registered Claim Names defined by RFC 7519 section
// 4.1. Each protocol embedding it determines which claims are required.
type JWTClaims struct {
	Issuer    string    `json:"iss,omitempty"`
	Subject   string    `json:"sub,omitempty"`
	Audience  Audience  `json:"aud,omitempty"`
	Expiry    *UnixTime `json:"exp,omitempty"`
	NotBefore *UnixTime `json:"nbf,omitempty"`
	IssuedAt  *UnixTime `json:"iat,omitempty"`
	ID        string    `json:"jti,omitempty"`
}

// AddressClaim is the structured postal address defined by OpenID Connect
// Core section 5.1.1.
type AddressClaim struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
}

// UnixTime is a JWT time claim expressed as seconds since the Unix epoch.
type UnixTime int64

// NewUnixTime converts time into UnixTime.
func NewUnixTime(value time.Time) *UnixTime {
	if value.IsZero() {
		return nil
	}
	current := UnixTime(value.Unix())
	return &current
}

// Time converts UnixTime into time.Time.
func (d *UnixTime) Time() time.Time {
	if d == nil {
		return time.Time{}
	}
	return time.Unix(int64(*d), 0)
}

// MarshalJSON encodes UnixTime as a JSON number.
func (d UnixTime) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(d), 10)), nil
}

// UnmarshalJSON decodes a JWT time claim. Fractional seconds are accepted and
// truncated because UnixTime stores seconds.
func (d *UnixTime) UnmarshalJSON(data []byte) error {
	value, err := strconv.ParseFloat(string(data), 64)
	if err != nil {
		return err
	}
	*d = UnixTime(value)
	return nil
}

// Audience is a JWT aud claim. Its JSON representation can be one string or
// an array of strings.
type Audience []string

// UnmarshalJSON decodes either representation of an audience claim.
func (a *Audience) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*a = Audience{value}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(a))
}

// MarshalJSON uses the compact string representation for one audience.
func (a Audience) MarshalJSON() ([]byte, error) {
	if len(a) == 1 {
		return json.Marshal(a[0])
	}
	return json.Marshal([]string(a))
}

// DecodeClaims decodes the verified ID Token claims into target.
func (t *IDToken) DecodeClaims(target any) error {
	return json.Unmarshal(t.claims, target)
}

// UserInfo is a verified OpenID Connect UserInfo response.
type UserInfo struct {
	UserInfoClaims

	claims json.RawMessage
}

// UserInfoClaims contains the JWT registered claims and Standard Claims used
// by UserInfo Responses. OpenID Connect Core sections 5.1 and 5.3.2 define
// the subject and profile claims; signed responses also use issuer and
// audience. Providers may include additional claims, which callers can decode
// separately from UserInfo.
type UserInfoClaims struct {
	JWTClaims
	Name                string        `json:"name,omitempty"`
	GivenName           string        `json:"given_name,omitempty"`
	FamilyName          string        `json:"family_name,omitempty"`
	MiddleName          string        `json:"middle_name,omitempty"`
	Nickname            string        `json:"nickname,omitempty"`
	PreferredUsername   string        `json:"preferred_username,omitempty"`
	Profile             string        `json:"profile,omitempty"`
	Picture             string        `json:"picture,omitempty"`
	Website             string        `json:"website,omitempty"`
	Email               string        `json:"email,omitempty"`
	EmailVerified       *bool         `json:"email_verified,omitempty"`
	Gender              string        `json:"gender,omitempty"`
	Birthdate           string        `json:"birthdate,omitempty"`
	ZoneInfo            string        `json:"zoneinfo,omitempty"`
	Locale              string        `json:"locale,omitempty"`
	PhoneNumber         string        `json:"phone_number,omitempty"`
	PhoneNumberVerified *bool         `json:"phone_number_verified,omitempty"`
	Address             *AddressClaim `json:"address,omitempty"`
	UpdatedAt           *UnixTime     `json:"updated_at,omitempty"`
}

// DecodeClaims decodes the verified UserInfo claims into target.
func (u *UserInfo) DecodeClaims(target any) error {
	return json.Unmarshal(u.claims, target)
}

// AccessToken contains verified JWT or introspection access-token claims.
// Authorization decisions remain the resource server's responsibility.
type AccessToken struct {
	Issuer   string
	Subject  string
	Audience []string
	Expiry   time.Time
	IssuedAt time.Time
	ID       string
	ClientID string
	Username string
	Scopes   []string

	claims json.RawMessage
}

// DecodeClaims decodes the verified access-token claims into target.
func (t *AccessToken) DecodeClaims(target any) error {
	return json.Unmarshal(t.claims, target)
}
