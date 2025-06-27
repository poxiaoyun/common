package api

import (
	"net/http"
	"net/url"
	"strings"
)

type RequestHeaderAuthenticatorOptions struct {
	NameHeader        string `json:"nameHeader,omitempty" description:"the name of the header to use for authentication"`
	GroupsHeader      string `json:"groupsHeader,omitempty" description:"the name of the header to use for groups"`
	ExtraHeaderPrefix string `json:"extraHeaderPrefix,omitempty" description:"the prefix of the header to use for extra attributes"`
}

func NewDefaultRequestHeaderAuthenticatorOptions() *RequestHeaderAuthenticatorOptions {
	return &RequestHeaderAuthenticatorOptions{
		NameHeader:        "X-Remote-User",
		GroupsHeader:      "X-Remote-Group",
		ExtraHeaderPrefix: "X-Remote-Extra-",
	}
}

type RequestHeaderAuthenticator struct {
	Options *RequestHeaderAuthenticatorOptions
}

func NewRequestHeaderAuthenticator(opts *RequestHeaderAuthenticatorOptions) *RequestHeaderAuthenticator {
	return &RequestHeaderAuthenticator{Options: opts}
}

func (a *RequestHeaderAuthenticator) Authenticate(r *http.Request) (*AuthenticateInfo, error) {
	header := r.Header
	name := header.Get(a.Options.NameHeader)
	if name == "" {
		return nil, ErrNotProvided
	}
	groups := header.Values(a.Options.GroupsHeader)
	extra := make(map[string][]string)
	for key, values := range header {
		cut, ok := strings.CutPrefix(key, a.Options.ExtraHeaderPrefix)
		if !ok || cut == "" {
			continue
		}
		cut = unescapeExtraKey(strings.ToLower(cut))
		extra[cut] = append(extra[cut], values...)
	}
	return &AuthenticateInfo{User: UserInfo{Name: name, Groups: groups, Extra: extra}}, nil
}

func unescapeExtraKey(encodedKey string) string {
	key, err := url.PathUnescape(encodedKey) // Decode %-encoded bytes.
	if err != nil {
		return encodedKey // Always record extra strings, even if malformed/unencoded.
	}
	return key
}
