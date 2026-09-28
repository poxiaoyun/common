package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/rest/proxy"
)

func TestProxyPreservesCapturedPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, r.RequestURI) }))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL + "/base")
	if err != nil {
		t.Fatal(err)
	}
	handler := api.New().
		Group(api.NewGroup("").
			Route(api.GET("/proxy/{path...}").
				To(func(w http.ResponseWriter, r *http.Request) {
					proxy.Proxy{ClientConfig: &httpclient.ClientConfig{Server: target}, RequestPath: "/" + api.Path(r, "path", "")}.ServeHTTP(w, r)
				}))).
		Build()
	for _, suffix := range []string{"/a%2fb", "/%252F", "/a//b/../c/?x=a+b&x=%2F"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/proxy"+suffix, nil))
		if response.Code != http.StatusOK || response.Body.String() != "/base"+suffix {
			t.Fatalf("%s: status=%d, body=%q", suffix, response.Code, response.Body.String())
		}
	}
}

func TestMultiHopProxyKeepsExternalPrefix(t *testing.T) {
	const prefix = "/api/moha/organizations/demo/spaces/chat/proxy"
	const kubePrefix = "/api/v1/namespaces/work/services/http:web:80/proxy"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Uri") != prefix+"/page?q=a%2Fb" || r.Host != "console.example.test" {
			t.Errorf("lost external request: host=%s uri=%s", r.Host, r.Header.Get("X-Forwarded-Uri"))
		}
		if r.URL.Query().Get("redirect") != "" {
			w.Header().Set("Location", kubePrefix+prefix+"/login?q=a%2Fb")
			w.WriteHeader(302)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<script src="`+kubePrefix+prefix+`/assets/app.js"></script><a href="`+kubePrefix+`/other">next</a>`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL + kubePrefix)
	middle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.Proxy{ClientConfig: &httpclient.ClientConfig{Server: target}, RemovePrefix: kubePrefix, RequestPath: "/page"}.ServeHTTP(w, r)
	}))
	defer middle.Close()
	middleURL, _ := url.Parse(middle.URL + "/internal")
	for _, query := range []string{"q=a%2Fb", "q=a%2Fb&redirect=1"} {
		req := httptest.NewRequest("GET", prefix+"/page?"+query, nil)
		req.Header.Set("X-Forwarded-Uri", prefix+"/page?q=a%2Fb")
		req.Header.Set("X-Forwarded-Prefix", prefix)
		req.Header.Set("X-Forwarded-Host", "console.example.test")
		req.Header.Set("X-Forwarded-Proto", "https")
		out := httptest.NewRecorder()
		proxy.Proxy{ClientConfig: &httpclient.ClientConfig{Server: middleURL}, RemovePrefix: "/internal", RequestPath: "/page"}.ServeHTTP(out, req)
		if query == "q=a%2Fb" {
			want := `<script src="` + prefix + `/assets/app.js"></script><a href="` + prefix + `/other">next</a>`
			if out.Body.String() != want {
				t.Fatalf("HTML = %s", out.Body.String())
			}
		} else if out.Header().Get("Location") != prefix+"/login?q=a%2Fb" {
			t.Fatalf("Location = %s", out.Header().Get("Location"))
		}
	}
}
