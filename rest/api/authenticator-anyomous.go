package api

import (
	"context"
	"net/http"

	"golang.org/x/crypto/ssh"
)

func NewAnonymousAuthenticator() *AnonymousAuthenticator {
	return &AnonymousAuthenticator{}
}

var AnonymousUserInfo = &AuthenticateInfo{
	User: UserInfo{
		ID:     AnonymousUser,
		Name:   AnonymousUser,
		Groups: []string{AnonymousUser},
	},
}

type AnonymousAuthenticator struct{}

var (
	_ Authenticator      = &AnonymousAuthenticator{}
	_ TokenAuthenticator = &AnonymousAuthenticator{}
	_ BasicAuthenticator = &AnonymousAuthenticator{}
	_ SSHAuthenticator   = &AnonymousAuthenticator{}
)

func (a AnonymousAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
	return AnonymousUserInfo, nil
}

func (a AnonymousAuthenticator) AuthenticateToken(ctx context.Context, token string) (*AuthenticateInfo, error) {
	return AnonymousUserInfo, nil
}

func (a AnonymousAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticateInfo, error) {
	return AnonymousUserInfo, nil
}

func (a AnonymousAuthenticator) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticateInfo, error) {
	return AnonymousUserInfo, nil
}
