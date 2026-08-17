package api

import (
	"context"
	"net/http"

	"golang.org/x/crypto/ssh"
)

func NewAnonymousAuthenticator() *AnonymousAuthenticator {
	return &AnonymousAuthenticator{}
}

func anonymousAuthenticationInfo() *AuthenticationInfo {
	return &AuthenticationInfo{Subject: Subject{
		ID:     AnonymousSubjectID,
		Name:   AnonymousSubjectID,
		Groups: []string{AnonymousSubjectID},
	}}
}

type AnonymousAuthenticator struct{}

var (
	_ Authenticator      = &AnonymousAuthenticator{}
	_ TokenAuthenticator = &AnonymousAuthenticator{}
	_ BasicAuthenticator = &AnonymousAuthenticator{}
	_ SSHAuthenticator   = &AnonymousAuthenticator{}
)

func (a AnonymousAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticationInfo, error) {
	return anonymousAuthenticationInfo(), nil
}

func (a AnonymousAuthenticator) AuthenticateToken(ctx context.Context, token string) (*AuthenticationInfo, error) {
	return anonymousAuthenticationInfo(), nil
}

func (a AnonymousAuthenticator) AuthenticateBasic(ctx context.Context, username, password string) (*AuthenticationInfo, error) {
	return anonymousAuthenticationInfo(), nil
}

func (a AnonymousAuthenticator) AuthenticatePublicKey(ctx context.Context, pubkey ssh.PublicKey) (*AuthenticationInfo, error) {
	return anonymousAuthenticationInfo(), nil
}
