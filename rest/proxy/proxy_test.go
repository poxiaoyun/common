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
