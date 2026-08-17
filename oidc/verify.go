package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// IDTokenVerifier verifies ID Tokens issued by one OpenID Provider.
type IDTokenVerifier struct {
	Issuer       string
	ClientSecret string
	Audiences    []string
	Algorithms   []string
	Keys         *KeySet
}

// IDTokenChecks binds an ID Token to the authorization values that caused it
// to be issued. Empty fields mean the value was not part of the flow.
type IDTokenChecks struct {
	Nonce       string
	AccessToken string
}

// VerifyIDToken validates an ID Token against the discovered provider and the
// client's trusted audiences.
func (c *Client) VerifyIDToken(ctx context.Context, raw string, checks IDTokenChecks) (*IDToken, error) {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	verifier, err := configuration.GetIDTokenVerifier()
	if err != nil {
		return nil, err
	}
	return verifier.Verify(ctx, raw, checks)
}

// NewIDTokenVerifier constructs an ID Token verifier from one provider
// metadata snapshot, the relying party's options, and a reusable KeySet.
func NewIDTokenVerifier(options ClientOptions, metadata OpenIDProviderMetadata, keys *KeySet) (*IDTokenVerifier, error) {
	if len(metadata.IDTokenSigningAlgValuesSupported) == 0 {
		return nil, fmt.Errorf("oidc: provider does not advertise ID Token signing algorithms")
	}
	requiresJWKS := slices.ContainsFunc(metadata.IDTokenSigningAlgValuesSupported, func(algorithm string) bool {
		return !strings.HasPrefix(algorithm, "HS")
	})
	if requiresJWKS && metadata.JWKSURI == "" {
		return nil, fmt.Errorf("%w: JWKS", ErrUnsupportedEndpoint)
	}
	audiences := options.IDTokenAudiences
	if len(audiences) == 0 {
		audiences = []string{options.Authentication.ClientID}
	}
	return &IDTokenVerifier{
		Issuer:       metadata.Issuer,
		ClientSecret: options.Authentication.ClientSecret,
		Audiences:    audiences,
		Algorithms:   metadata.IDTokenSigningAlgValuesSupported,
		Keys:         keys,
	}, nil
}

// Verify validates an ID Token using this verifier's provider configuration.
func (v *IDTokenVerifier) Verify(ctx context.Context, raw string, checks IDTokenChecks) (*IDToken, error) {
	allowed := make([]jose.SignatureAlgorithm, len(v.Algorithms))
	for index, algorithm := range v.Algorithms {
		allowed[index] = jose.SignatureAlgorithm(algorithm)
	}
	signed, err := jose.ParseSignedCompact(raw, allowed)
	if err != nil {
		return nil, err
	}
	header := signed.Signatures[0].Header
	typeValue, _ := header.ExtraHeaders[jose.HeaderType].(string)
	if strings.EqualFold(typeValue, "at+jwt") || strings.EqualFold(typeValue, "application/at+jwt") {
		return nil, fmt.Errorf("oidc: access token cannot be used as an ID Token")
	}
	var payload []byte
	if strings.HasPrefix(header.Algorithm, "HS") {
		payload, err = signed.Verify([]byte(v.ClientSecret))
		if err != nil {
			return nil, fmt.Errorf("oidc: JWT signature is invalid: %w", err)
		}
	} else {
		verifiedPayload, _, verifyErr := v.Keys.Verify(ctx, raw, v.Algorithms)
		if verifyErr != nil {
			return nil, verifyErr
		}
		payload = verifiedPayload
	}
	claims := IDTokenClaims{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	if claims.Issuer == "" || claims.Subject == "" || len(claims.Audience) == 0 || claims.Expiry == nil || claims.IssuedAt == nil {
		return nil, fmt.Errorf("oidc: ID Token is missing required claims")
	}
	if claims.Issuer != v.Issuer {
		return nil, fmt.Errorf("oidc: invalid ID Token issuer")
	}
	for _, audience := range claims.Audience {
		if !slices.Contains(v.Audiences, audience) {
			return nil, fmt.Errorf("oidc: untrusted ID Token audience %q", audience)
		}
	}
	now := time.Now()
	if !now.Before(claims.Expiry.Time()) {
		return nil, fmt.Errorf("oidc: ID Token is expired")
	}
	if now.Before(claims.IssuedAt.Time()) {
		return nil, fmt.Errorf("oidc: ID Token was issued in the future")
	}
	if len(claims.Audience) > 1 && claims.AuthorizedParty == "" {
		return nil, fmt.Errorf("oidc: ID Token with multiple audiences is missing authorized party")
	}
	if claims.AuthorizedParty != "" && !slices.Contains(v.Audiences, claims.AuthorizedParty) {
		return nil, fmt.Errorf("oidc: invalid ID Token authorized party")
	}
	if checks.Nonce != "" && subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(checks.Nonce)) != 1 {
		return nil, fmt.Errorf("oidc: ID Token nonce mismatch")
	}
	// OpenID Connect Core 1.0 sections 3.1.3.8 and 3.2.2.9 define
	// at_hash validation. When the ID Token contains at_hash and its matching
	// Access Token is available, verify their cryptographic binding to detect
	// Access Token substitution.
	if claims.AccessTokenHash != "" && checks.AccessToken != "" {
		if err := VerifyHash(checks.AccessToken, claims.AccessTokenHash, header.Algorithm); err != nil {
			return nil, fmt.Errorf("oidc: invalid ID Token access token hash: %w", err)
		}
	}
	return &IDToken{Raw: raw, IDTokenClaims: claims, claims: payload}, nil
}

// TokenHash computes the OIDC left-half hash of value using the hash associated
// with the JWT signing algorithm.
func TokenHash(value, algorithm string) (string, error) {
	var digest hash.Hash
	switch algorithm {
	case "HS256", "RS256", "ES256", "PS256":
		digest = sha256.New()
	case "HS384", "RS384", "ES384", "PS384":
		digest = sha512.New384()
	case "HS512", "RS512", "ES512", "PS512", "EdDSA":
		digest = sha512.New()
	default:
		return "", fmt.Errorf("unsupported signing algorithm %q", algorithm)
	}
	_, _ = digest.Write([]byte(value))
	sum := digest.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:len(sum)/2]), nil
}

// VerifyHash verifies an OIDC token hash using the hash associated with the JWT
// signing algorithm.
func VerifyHash(value, expected, algorithm string) error {
	actual, err := TokenHash(value, algorithm)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("hash mismatch")
	}
	return nil
}
