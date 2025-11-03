package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"xiaoshiai.cn/common/httpclient"
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

// RequestHeaderAuthenticator is an authenticator that uses request headers to authenticate users.
//
// use [SetAuthProxyHeaders] to set the headers.
//
// Or use [SetAuthProxyHeadersFromContext] to set the headers from an existing authentication context.
type RequestHeaderAuthenticator struct {
	Options *RequestHeaderAuthenticatorOptions
}

func NewRequestHeaderAuthenticator(opts *RequestHeaderAuthenticatorOptions) *RequestHeaderAuthenticator {
	return &RequestHeaderAuthenticator{Options: opts}
}

func (a *RequestHeaderAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (*AuthenticateInfo, error) {
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

func SetAuthProxyHeadersFromContext(ctx context.Context, req *http.Request) {
	info := AuthenticateFromContext(ctx)
	SetAuthProxyHeaders(req, info.User.Name, info.User.Groups, info.User.Extra)
}

// SetAuthProxyHeaders sets the authentication proxy headers on the given request.
// It sets the X-Remote-User, X-Remote-Group, and X-Remote-Extra-* headers
// It consumes by [RequestHeaderAuthenticator] to reconstruct the user info.
func SetAuthProxyHeaders(req *http.Request, username string, groups []string, extra map[string][]string) {
	httpclient.SetAuthProxyHeaders(req, username, groups, extra)
}
