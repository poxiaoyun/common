package authn

import (
	"context"

	"xiaoshiai.cn/common/rest/api"
)

var _ api.TokenAuthenticator = &SelfAuthAuthenticator{}

func NewSelfTokenAuthenticator(provider AuthProvider) (*SelfAuthAuthenticator, error) {
	return &SelfAuthAuthenticator{Provider: provider}, nil
}

type SelfAuthAuthenticator struct {
	Provider AuthProvider
}

// Authenticate implements api.TokenAuthenticator.
func (s *SelfAuthAuthenticator) Authenticate(ctx context.Context, token string) (*api.AuthenticateInfo, error) {
	session, err := s.Provider.GetCurrentProfile(ctx, token)
	if err != nil {
		return nil, err
	}
	info := &api.AuthenticateInfo{
		User: api.UserInfo{
			ID:            session.User.Subject,
			Name:          session.User.Name,
			Email:         session.User.Email,
			EmailVerified: session.User.EmailVerified,
			Groups:        session.User.Groups,
		},
	}
	return info, nil
}

type ApikeyAuthAuthenticator struct {
	Provider AuthProvider
}

func NewAPIKeyAuthenticator(provider AuthProvider) *ApikeyAuthAuthenticator {
	return &ApikeyAuthAuthenticator{Provider: provider}
}

func (a *ApikeyAuthAuthenticator) Authenticate(ctx context.Context, username, password string) (*api.AuthenticateInfo, error) {
	user, err := a.Provider.CheckAPIKey(ctx, APIKey{AccessKey: username, SecretKey: password})
	if err != nil {
		return nil, err
	}
	info := &api.AuthenticateInfo{
		User: api.UserInfo{
			ID:            user.Subject,
			Name:          user.Name,
			Email:         user.Email,
			EmailVerified: user.EmailVerified,
			Groups:        user.Groups,
		},
	}
	return info, nil
}
