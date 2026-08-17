package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
)

// GetUserInfo obtains claims using accessToken and binds the response to the
// subject from its verified ID Token. JSON and signed-JWT responses are
// supported.
func (c *Client) GetUserInfo(ctx context.Context, accessToken, idTokenSubject string) (*UserInfo, error) {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	metadata := configuration.Metadata
	if metadata.UserInfoEndpoint == "" {
		return nil, fmt.Errorf("%w: UserInfo", ErrUnsupportedEndpoint)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := configuration.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, DecodeEndpointResponse(response, &struct{}{})
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	var payload json.RawMessage
	claims := UserInfoClaims{}
	switch mediaType {
	case "application/jwt":
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		algorithms := metadata.UserInfoSigningAlgValuesSupported
		allowed := make([]jose.SignatureAlgorithm, len(algorithms))
		for index, algorithm := range algorithms {
			allowed[index] = jose.SignatureAlgorithm(algorithm)
		}
		signed, err := jose.ParseSignedCompact(string(raw), allowed)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(signed.Signatures[0].Header.Algorithm, "HS") {
			payload, err = signed.Verify([]byte(configuration.Authentication.ClientSecret))
			if err != nil {
				return nil, fmt.Errorf("oidc: UserInfo signature is invalid: %w", err)
			}
		} else {
			if metadata.JWKSURI == "" {
				return nil, fmt.Errorf("%w: JWKS", ErrUnsupportedEndpoint)
			}
			payload, _, err = configuration.KeySet.Verify(ctx, string(raw), algorithms)
			if err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal(payload, &claims); err != nil {
			return nil, err
		}
		if claims.Issuer != metadata.Issuer || !slices.Contains(claims.Audience, configuration.Authentication.ClientID) {
			return nil, fmt.Errorf("oidc: signed UserInfo issuer or audience mismatch")
		}
	case "application/json":
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &claims); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("oidc: unsupported UserInfo content type %q", mediaType)
	}
	// OpenID Connect Core 1.0 section 5.3.2 requires every UserInfo
	// Response to contain sub and requires it to exactly match the ID Token
	// sub. A mismatch can indicate token substitution, so none of the
	// UserInfo values may be used.
	if claims.Subject == "" {
		return nil, fmt.Errorf("oidc: UserInfo response is missing subject")
	}
	if claims.Subject != idTokenSubject {
		return nil, fmt.Errorf("oidc: UserInfo subject does not match ID Token")
	}
	return &UserInfo{UserInfoClaims: claims, claims: payload}, nil
}
