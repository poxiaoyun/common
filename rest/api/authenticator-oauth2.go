package api

import (
	"context"
	"errors"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/oidc"
)

// OAuth2AccessTokenAuthenticator validates OAuth 2.0 access tokens and maps
// their verified claims to the canonical authentication information.
type OAuth2AccessTokenAuthenticator struct {
	// Client verifies access tokens against one lazily discovered provider.
	Client *oidc.Client
}

// NewOAuth2AccessTokenAuthenticator adapts an OIDC client configured for
// access-token validation to the REST authentication interface.
func NewOAuth2AccessTokenAuthenticator(client *oidc.Client) *OAuth2AccessTokenAuthenticator {
	return &OAuth2AccessTokenAuthenticator{Client: client}
}

// AuthenticateToken verifies raw and returns its subject, current actor, and
// access constraints. OAuth client metadata remains in the protocol result.
func (a *OAuth2AccessTokenAuthenticator) AuthenticateToken(ctx context.Context, raw string) (*Authentication, error) {
	token, err := a.Client.VerifyAccessToken(ctx, raw)
	if err != nil {
		if errors.Is(err, oidc.ErrInvalidAccessToken) {
			log.FromContext(ctx).Error(err, "OAuth 2.0 access token rejected")
			return nil, NewUnauthorizedChallengeError(`Bearer error="invalid_token"`, "Unauthorized")
		}
		return nil, err
	}
	info := Authentication{
		Subject: authn.Subject{ID: token.Subject, Name: token.Username},
		Token: &authn.TokenInfo{
			Audiences: token.Audience,
			Scopes:    token.Scopes,
		},
	}
	if token.Actor != nil {
		info.Actor = &authn.Subject{ID: token.Actor.Subject}
	}
	return &info, nil
}

var _ TokenAuthenticator = &OAuth2AccessTokenAuthenticator{}
