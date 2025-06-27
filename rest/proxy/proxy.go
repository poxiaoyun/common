package proxy

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	liberrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/rest/api"
)

type Proxy struct {
	// ClientConfig is the client configuration to use for the proxy
	ClientConfig *httpclient.ClientConfig
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
			pr.SetURL(p.ClientConfig.Server)
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
	rp.Transport = &Transport{
		PathPrepend:  prefix,
		PathRemove:   p.RemovePrefix,
		RoundTripper: rp.Transport,
	}
	rp.ServeHTTP(w, req)
}

type ErrorResponser struct{}

func (ErrorResponser) Error(w http.ResponseWriter, req *http.Request, err error) {
	dnse := &net.DNSError{}
	if errors.As(err, &dnse) {
		err = liberrors.NewServiceUnavailable(err.Error())
	}
	statuserr := &apierrors.StatusError{}
	if errors.As(err, &statuserr) {
		api.Error(w, liberrors.NewCustomError(
			int(statuserr.ErrStatus.Code),
			liberrors.StatusReason(statuserr.ErrStatus.Reason),
			statuserr.ErrStatus.Message))
		return
	}
	api.Error(w, err)
	log.Error(err, "proxy error", "url", req.URL.String())
}
