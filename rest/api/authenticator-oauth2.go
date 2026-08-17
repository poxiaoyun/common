package api

import (
	"context"
	"errors"
	"net/http"

	"xiaoshiai.cn/common/oidc"
)

const (
	// IAMPrincipalTypeExtra carries the IAM principal type used to select the
	// authorization policy for an authenticated identity.
	IAMPrincipalTypeExtra = "iam.principal.type"
	// OAuth2ClientIDExtra carries the OAuth 2.0 client_id associated with an
	// access token.
	OAuth2ClientIDExtra = "oauth2.client_id"
	// OAuth2ScopeExtra carries the OAuth 2.0 scopes granted to an access token.
	OAuth2ScopeExtra = "oauth2.scope"
	// OAuth2ClientPrincipalType identifies a client_credentials service
	// principal.
	OAuth2ClientPrincipalType = "oauth2_client"
)

// OAuth2AccessTokenAuthenticator validates IAM service access tokens and maps
// their verified claims to the authentication information consumed by API
// authorizers.
type OAuth2AccessTokenAuthenticator struct {
	// Client verifies access tokens against one lazily discovered provider.
	Client *oidc.Client
}

// NewOAuth2AccessTokenAuthenticator adapts an OIDC client configured for
// access-token validation to the REST authentication interface.
func NewOAuth2AccessTokenAuthenticator(client *oidc.Client) *OAuth2AccessTokenAuthenticator {
	return &OAuth2AccessTokenAuthenticator{Client: client}
}

// OAuth2BearerAuthenticationError writes the RFC 6750 Bearer challenge for a
// failed protected-resource request.
func OAuth2BearerAuthenticationError(w http.ResponseWriter, _ *http.Request, err error) {
	challenge := "Bearer"
	if errors.Is(err, oidc.ErrInvalidAccessToken) {
		challenge += ` error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	Unauthorized(w, "Unauthorized")
}

// AuthenticateToken verifies raw and returns its service principal, client ID,
// granted scopes, and the audience validated by this authenticator.
func (a *OAuth2AccessTokenAuthenticator) AuthenticateToken(ctx context.Context, raw string) (*AuthenticateInfo, error) {
	token, err := a.Client.VerifyAccessToken(ctx, raw)
	if err != nil {
		return nil, err
	}
	return &AuthenticateInfo{
		Audiences: token.Audience,
		User: UserInfo{
			ID:   token.Subject,
			Name: token.Issuer + "#" + token.Subject,
			Extra: map[string][]string{
				IAMPrincipalTypeExtra: {OAuth2ClientPrincipalType},
				OAuth2ClientIDExtra:   {token.ClientID},
				OAuth2ScopeExtra:      token.Scopes,
			},
		},
	}, nil
}

var _ TokenAuthenticator = &OAuth2AccessTokenAuthenticator{}
