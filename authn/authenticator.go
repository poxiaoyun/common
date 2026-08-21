package authn

import (
	"context"

	"xiaoshiai.cn/common/rest/api"
)

var _ api.TokenAuthenticator = &SelfTokenAuthenticator{}
var _ api.BasicAuthenticator = &APIKeyAuthenticator{}

// NewSelfTokenAuthenticator returns an authenticator backed by provider
// sessions.
func NewSelfTokenAuthenticator(provider AuthProvider) (*SelfTokenAuthenticator, error) {
	return &SelfTokenAuthenticator{Provider: provider}, nil
}

// SelfTokenAuthenticator authenticates provider session tokens.
type SelfTokenAuthenticator struct {
	Provider AuthProvider
}

// AuthenticateToken implements api.TokenAuthenticator.
func (s *SelfTokenAuthenticator) AuthenticateToken(ctx context.Context, token string) (*api.AuthenticationInfo, error) {
	session, err := s.Provider.GetCurrentProfile(ctx, token)
	if err != nil {
		return nil, err
	}
	info := &api.AuthenticationInfo{
		Subject: api.Subject{
			ID:            session.User.Subject,
			Name:          session.User.Name,
			DisplayName:   session.User.DisplayName,
			Email:         session.User.Email,
			EmailVerified: session.User.EmailVerified,
			Groups:        session.User.Groups,
		},
	}
	return info, nil
}

// APIKeyAuthenticator authenticates API key pairs.
type APIKeyAuthenticator struct {
	Provider AuthProvider
}

// NewAPIKeyAuthenticator returns an API key authenticator backed by provider.
func NewAPIKeyAuthenticator(provider AuthProvider) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{Provider: provider}
}

// AuthenticateBasic authenticates an access key and secret key pair.
func (a *APIKeyAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*api.AuthenticationInfo, error) {
	user, err := a.Provider.CheckAPIKey(ctx, APIKey{AccessKey: username, SecretKey: password})
	if err != nil {
		return nil, err
	}
	info := &api.AuthenticationInfo{
		Subject: api.Subject{
			ID:            user.Subject,
			Name:          user.Name,
			DisplayName:   user.DisplayName,
			Email:         user.Email,
			EmailVerified: user.EmailVerified,
			Groups:        user.Groups,
		},
	}
	return info, nil
}
