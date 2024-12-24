package httpclient

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	libproxy "xiaoshiai.cn/common/rest/proxy"
)

type Proxy struct {
	// ClientConfig is the client configuration to use for the proxy
	ClientConfig *ClientConfig
	// RemovePrefix is the prefix to remove from the request path
	// it only useful when request proxy via a kubernetes service proxy,
	// it add a useless prefix /v1/namespaces/{namespace}/services/{service}:{port}/proxy/
	// we need to remove it, set RemovePrefix to '/v1/namespaces/{namespace}/services/{service}:{port}/proxy'
	RemovePrefix string
	// RequestPath is the path to proxy to the backend
	// real request path will be clientconfig.Server.Path + RequestPath
	RequestPath string
	// ProxyPrefix is the prefix to add to the request path
	// eg. /proxy/v1/info
	// when backend receives the request, it will only see /v1/info
	// this is useful when you want to add a prefix to the modified html
	// set ProxyPrefix to '/proxy'
	ProxyPrefix string
}

func (p Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requestpath := p.RequestPath
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Path = requestpath
			pr.SetURL(&p.ClientConfig.Server)
		},
		Transport: p.ClientConfig.RoundTripper,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadGateway)
		},
	}
	prefix := strings.TrimSuffix(req.URL.Path, requestpath)
	if proxyRequestURI, _ := url.ParseRequestURI(req.Header.Get("X-Forwarded-Uri")); proxyRequestURI != nil {
		// is this request from a proxy?
		// if request is from a proxy, the X-Forwarded-Uri header will be set
		// example: X-Forwarded-Uri: /api/v1/namespaces/default/pods/foo/vnc/somepath
		// we need use the path from the X-Forwarded-Uri header to set the prefix
		// prefix = "/api/v1/namespaces/default/pods/foo"
		prefix = strings.TrimSuffix(proxyRequestURI.Path, requestpath)
	} else {
		// /proxy is our front end proxy path
		prefix = filepath.Join(p.ProxyPrefix, prefix)
	}
	rp.Transport = &libproxy.Transport{
		PathPrepend:  prefix,
		PathRemove:   p.RemovePrefix,
		RoundTripper: rp.Transport,
	}
	rp.ServeHTTP(w, req)
}
