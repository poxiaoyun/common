package oidc

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// RevokeToken asks the provider to revoke an access or refresh token.
func (c *Client) RevokeToken(ctx context.Context, token string, hint TokenTypeHint) error {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return err
	}
	authentication, err := configuration.GetRevocationAuthentication()
	if err != nil {
		return err
	}
	form := url.Values{"token": {token}}
	if hint != "" {
		form.Set("token_type_hint", string(hint))
	}
	if authentication.Method == ClientAuthSecretPost {
		form.Set("client_id", authentication.ClientID)
		form.Set("client_secret", authentication.ClientSecret)
	}
	if authentication.Method == ClientAuthNone {
		form.Set("client_id", authentication.ClientID)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, configuration.Metadata.RevocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if authentication.Method == ClientAuthSecretBasic {
		request.SetBasicAuth(url.QueryEscape(authentication.ClientID), url.QueryEscape(authentication.ClientSecret))
	}
	response, err := configuration.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return DecodeEndpointResponse(response, &struct{}{})
	}
	_ = response.Body.Close()
	return nil
}

// RPInitiatedLogout contains the provider logout URL and the state that must be
// checked when the provider redirects back.
type RPInitiatedLogout struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// BeginRPInitiatedLogout creates an RP-Initiated Logout request using a
// previously issued ID Token as id_token_hint. The caller manages its own local
// session.
func (c *Client) BeginRPInitiatedLogout(ctx context.Context, idToken *IDToken, postLogoutRedirectURL string) (RPInitiatedLogout, error) {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return RPInitiatedLogout{}, err
	}
	configuredEndpoint, err := configuration.GetEndSessionEndpoint()
	if err != nil {
		return RPInitiatedLogout{}, err
	}
	endpoint := *configuredEndpoint
	state := oauth2.GenerateVerifier()
	query := endpoint.Query()
	query.Set("id_token_hint", idToken.Raw)
	query.Set("post_logout_redirect_uri", postLogoutRedirectURL)
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()
	return RPInitiatedLogout{URL: endpoint.String(), State: state}, nil
}

// CompleteRPInitiatedLogout validates the state returned by the provider.
func (c *Client) CompleteRPInitiatedLogout(callback url.Values, expected RPInitiatedLogout) error {
	if callback.Get("state") != expected.State {
		return ErrStateMismatch
	}
	return nil
}
