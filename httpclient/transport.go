package httpclient

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type WrappedRoundTripper interface {
	WrappedRoundTripper() http.RoundTripper
}

const MaxidleConnsPerHost = 25

func NewDefaultHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: MaxidleConnsPerHost,
	}
}

func NewBearerTokenRoundTripper(token string, rt http.RoundTripper) http.RoundTripper {
	return &BearerTokenRoundTripper{token: token, transport: rt}
}

func NewBearerTokenFuncRoundTripper(tokenFunc func(r *http.Request) (string, error), rt http.RoundTripper) (http.RoundTripper, error) {
	if tokenFunc == nil {
		return nil, fmt.Errorf("tokenFunc is required")
	}
	return &BearerTokenRoundTripper{tokenFunc: tokenFunc, transport: rt}, nil
}

type BearerTokenRoundTripper struct {
	token     string
	tokenFunc func(r *http.Request) (string, error)
	transport http.RoundTripper
}

func (rt *BearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the request already has an Authorization header, we assume it is already authenticated.
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.transport.RoundTrip(req)
	}
	token := rt.token
	if rt.tokenFunc != nil {
		dynamicToken, err := rt.tokenFunc(req)
		if err != nil {
			return nil, err
		}
		token = dynamicToken
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.transport.RoundTrip(req)
}

func (rt *BearerTokenRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.transport }

type BasicAuthRoundTripper struct {
	username  string
	password  string
	transport http.RoundTripper
}

func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &BasicAuthRoundTripper{username: username, password: password, transport: rt}
}

func (rt *BasicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.transport.RoundTrip(req)
	}
	req.SetBasicAuth(rt.username, rt.password)
	return rt.transport.RoundTrip(req)
}

func (rt *BasicAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.transport }

func NewAuthProxyRoundTripper(username string, groups []string, extra map[string][]string, rt http.RoundTripper) http.RoundTripper {
	return &authProxyRoundTripper{
		username: username,
		groups:   groups,
		extra:    extra,
		rt:       rt,
	}
}

type authProxyRoundTripper struct {
	username string
	groups   []string
	extra    map[string][]string

	rt http.RoundTripper
}

func (rt *authProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	SetAuthProxyHeaders(req, rt.username, rt.groups, rt.extra)
	return rt.rt.RoundTrip(req)
}

// SetAuthProxyHeaders stomps the auth proxy header fields.  It mutates its argument.
func SetAuthProxyHeaders(req *http.Request, username string, groups []string, extra map[string][]string) {
	req.Header.Del("X-Remote-User")
	req.Header.Del("X-Remote-Group")
	for key := range req.Header {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower("X-Remote-Extra-")) {
			req.Header.Del(key)
		}
	}
	req.Header.Set("X-Remote-User", username)
	for _, group := range groups {
		req.Header.Add("X-Remote-Group", group)
	}
	for key, values := range extra {
		for _, value := range values {
			req.Header.Add("X-Remote-Extra-"+headerKeyEscape(key), value)
		}
	}
}

func headerKeyEscape(key string) string {
	buf := strings.Builder{}
	for i := 0; i < len(key); i++ {
		b := key[i]
		if shouldEscape(b) {
			// %-encode bytes that should be escaped:
			// https://tools.ietf.org/html/rfc3986#section-2.1
			fmt.Fprintf(&buf, "%%%02X", b)
			continue
		}
		buf.WriteByte(b)
	}
	return buf.String()
}

func shouldEscape(b byte) bool {
	// url.PathUnescape() returns an error if any '%' is not followed by two
	// hexadecimal digits, so we'll intentionally encode it.
	return !legalHeaderByte(b) || b == '%'
}

func legalHeaderByte(b byte) bool {
	return int(b) < len(legalHeaderKeyBytes) && legalHeaderKeyBytes[b]
}

// legalHeaderKeyBytes was copied from net/http/lex.go's isTokenTable.
// See https://httpwg.github.io/specs/rfc7230.html#rule.token.separators
var legalHeaderKeyBytes = [127]bool{
	'%':  true,
	'!':  true,
	'#':  true,
	'$':  true,
	'&':  true,
	'\'': true,
	'*':  true,
	'+':  true,
	'-':  true,
	'.':  true,
	'0':  true,
	'1':  true,
	'2':  true,
	'3':  true,
	'4':  true,
	'5':  true,
	'6':  true,
	'7':  true,
	'8':  true,
	'9':  true,
	'A':  true,
	'B':  true,
	'C':  true,
	'D':  true,
	'E':  true,
	'F':  true,
	'G':  true,
	'H':  true,
	'I':  true,
	'J':  true,
	'K':  true,
	'L':  true,
	'M':  true,
	'N':  true,
	'O':  true,
	'P':  true,
	'Q':  true,
	'R':  true,
	'S':  true,
	'T':  true,
	'U':  true,
	'W':  true,
	'V':  true,
	'X':  true,
	'Y':  true,
	'Z':  true,
	'^':  true,
	'_':  true,
	'`':  true,
	'a':  true,
	'b':  true,
	'c':  true,
	'd':  true,
	'e':  true,
	'f':  true,
	'g':  true,
	'h':  true,
	'i':  true,
	'j':  true,
	'k':  true,
	'l':  true,
	'm':  true,
	'n':  true,
	'o':  true,
	'p':  true,
	'q':  true,
	'r':  true,
	's':  true,
	't':  true,
	'u':  true,
	'v':  true,
	'w':  true,
	'x':  true,
	'y':  true,
	'z':  true,
	'|':  true,
	'~':  true,
}
