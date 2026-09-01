package api

import (
	"net/http"

	"xiaoshiai.cn/common/authn"
)

func NewAnonymousAuthenticator() *AnonymousAuthenticator {
	return &AnonymousAuthenticator{}
}

func anonymousAuthentication() *Authentication {
	return &Authentication{Subject: Subject{
		ID:     AnonymousSubjectID,
		Type:   authn.SubjectTypeAnonymous,
		Name:   AnonymousSubjectID,
		Groups: []string{AnonymousSubjectID},
	}}
}

type AnonymousAuthenticator struct{}

var _ HTTPAuthenticator = &AnonymousAuthenticator{}

// AuthenticateHTTP returns the anonymous authentication.
func (a AnonymousAuthenticator) AuthenticateHTTP(w http.ResponseWriter, r *http.Request) (*Authentication, error) {
	return anonymousAuthentication(), nil
}
