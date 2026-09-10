package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"xiaoshiai.cn/common/rest/proxy"
)

func TestTransportRedirectPrefixReplacement(t *testing.T) {
	for _, test := range []struct {
		name, prepend, remove, location, want string
	}{
		{"moha", "/api/moha", "/v1/", "/v1/login?next=a%2Fb#section", "/api/moha/login?next=a%2Fb#section"},
		{"airouter", "/api/airouter", "/api/", "/api/login", "/api/airouter/login"},
		{"already external", "/api/airouter", "/api/", "/api/airouter/login", "/api/airouter/login"},
		{"upstream prefix boundary", "/api/airouter", "/api/", "/apiary/login", "/api/airouter/apiary/login"},
		{"external prefix boundary", "/api/airouter", "/api/", "/api/airouter-other/login", "/api/airouter/airouter-other/login"},
		{"encoded separators", "/api/moha", "/v1/", "/v1/items/a%2Fb", "/api/moha/items/a%2Fb"},
		{"encoded prefix", "/api/moha", "/v1/", "/%761/items/a%2Fb", "/api/moha/items/a%2Fb"},
		{"encoded dot segments", "/api/moha", "/v1/", "/v1/a%2F..%2Fb", "/api/moha/a%2F..%2Fb"},
		{"literal dot segments", "/api/moha", "/v1/", "/v1/a/../b", "/api/moha/a/../b"},
		{"repeated separators", "/api/moha", "/v1/", "/v1/a//b/", "/api/moha/a//b/"},
		{"encoded trailing separator", "/api/moha", "/v1/", "/v1/a%2F", "/api/moha/a%2F"},
		{"trailing separator", "/api/airouter", "/api/", "/api/", "/api/airouter/"},
		{"empty external prefix", "", "/api/", "/api", "/"},
		{"root upstream", "/moha", "/", "/moha/login", "/moha/login"},
		{"other host", "/api/moha", "/v1/", "https://cdn.example.test/v1/image", "https://cdn.example.test/v1/image"},
		{"relative", "/api/moha", "/v1/", "../login", "../login"},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().
					Set("Location", test.location)
				w.WriteHeader(http.StatusFound)
			}))
			defer upstream.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL+"/page", nil)
			if err != nil {
				t.Fatal(err)
			}
			transport := &proxy.Transport{PathPrepend: test.prepend, PathRemove: test.remove}
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if got := response.Header.Get("Location"); got != test.want {
				t.Fatalf("Location = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTransportHTMLPrefixReplacement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().
			Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<a href="/api/login">login</a><a href="/api/airouter/existing">existing</a><a href="/api/a%2F..%2Fb">encoded</a><a href="../relative">relative</a><img src="https://cdn.example.test/api/image">`)
	}))
	defer upstream.Close()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL+"/api/page", nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &proxy.Transport{PathPrepend: "/api/airouter", PathRemove: "/api/"}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="/api/airouter/login"`,
		`href="/api/airouter/existing"`,
		`href="/api/airouter/a%2F..%2Fb"`,
		`href="../relative"`,
		`src="https://cdn.example.test/api/image"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("HTML missing %s: %s", want, body)
		}
	}
}

func TestRewritePath(t *testing.T) {
	for _, tt := range []struct{ input, remove, prepend, want string }{
		{"/api/items/a%2fb?q=a+b&q=%252F", "/api", "/v1/", "/v1/items/a%2fb?q=a+b&q=%252F"},
		{"/%61pi/items/%252F", "/api", "/", "/items/%252F"},
		{"/api//a/../b/", "/api", "/v1/", "/v1//a/../b/"},
		{"/apix/items", "/api", "/v1", "/v1/apix/items"},
		{"/api", "/api", "/", "/"},
		{"/api", "/api", "/v1/", "/v1/"},
		{"/api/", "/api", "", "/"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			target, err := url.Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			proxy.RewritePath(target, tt.remove, tt.prepend)
			if got := target.String(); got != tt.want {
				t.Fatalf("rewritten URL = %q, want %q", got, tt.want)
			}
		})
	}
}
