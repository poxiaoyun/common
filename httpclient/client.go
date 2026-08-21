package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/url"

	libtls "xiaoshiai.cn/common/tls"
)

// Options describes a remote HTTP endpoint, TLS, proxy, and authentication.
type Options struct {
	Server                string `json:"server,omitempty" description:"remote HTTP server address"`
	ProxyURL              string `json:"proxyURL,omitempty" description:"HTTP proxy server address"`
	Token                 string `json:"token,omitempty" config:"token,sensitive" description:"bearer token sent to the remote server"`
	Username              string `json:"username,omitempty" description:"basic authentication username"`
	Password              string `json:"password,omitempty" config:"password,sensitive" description:"basic authentication password"`
	CertFile              string `json:"certFile,omitempty" description:"path to the TLS client certificate"`
	KeyFile               string `json:"keyFile,omitempty" description:"path to the TLS client private key"`
	CAFile                string `json:"caFile,omitempty" description:"path to a CA certificate bundle"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty" description:"skip verification of the remote TLS certificate"`
}

// TransportConfig supplies runtime behavior for the underlying HTTP transport.
type TransportConfig struct {
	// DialContext replaces the network dialer used by both HTTP and WebSocket
	// clients built from the resulting ClientConfig. Nil uses net/http's default
	// dial behavior.
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// BuildClientConfig assembles the runtime client configuration.
func BuildClientConfig(ctx context.Context, options *Options, transportConfig TransportConfig) (*ClientConfig, error) {
	serverURL, err := url.Parse(options.Server)
	if err != nil {
		return nil, err
	}
	httptransport := NewDefaultHTTPTransport()
	httptransport.DialContext = transportConfig.DialContext
	// tls
	tlsconfig, err := libtls.NewDynamicTLSConfig(ctx, &libtls.DynamicTLSConfigOptions{
		CertFile:              options.CertFile,
		KeyFile:               options.KeyFile,
		CAFile:                options.CAFile,
		InsecureSkipTLSVerify: options.InsecureSkipTLSVerify,
	})
	if err != nil {
		return nil, err
	}
	httptransport.TLSClientConfig = tlsconfig
	// proxy
	if options.ProxyURL != "" {
		proxyURL, err := url.Parse(options.ProxyURL)
		if err != nil {
			return nil, err
		}
		httptransport.Proxy = http.ProxyURL(proxyURL)
	}
	tp := http.RoundTripper(httptransport)
	if options.Token != "" {
		tp = NewBearerTokenRoundTripper(options.Token, tp)
	}
	if options.Username != "" && options.Password != "" {
		tp = NewBasicAuthRoundTripper(options.Username, options.Password, tp)
	}
	return &ClientConfig{
		Server:       serverURL,
		RoundTripper: tp,
		DialContext:  transportConfig.DialContext,
		Proxy:        httptransport.Proxy,
	}, nil
}

type ClientConfig struct {
	// Server is the base URL used to resolve relative request paths.
	Server *url.URL
	// RoundTripper contains the configured TLS, proxy, and authentication layers
	// used by HTTP requests.
	RoundTripper http.RoundTripper
	// DialContext is exposed separately because WebSocket dialers do not execute
	// an HTTP RoundTripper.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// Proxy is exposed separately for the same reason as DialContext.
	Proxy func(*http.Request) (*url.URL, error)
}

type Client struct {
	Client       *http.Client
	Server       *url.URL
	RoundTripper http.RoundTripper
	OnRequest    func(req *http.Request) error
	OnResponse   func(req *http.Request, resp *http.Response) error
	Debug        bool
}

// NewClientFromOptions builds a Client with the transport described by options.
func NewClientFromOptions(ctx context.Context, options *Options) (*Client, error) {
	return NewClientFromOptionsWithTransport(ctx, options, nil)
}

// NewClientFromOptionsWithTransport builds the configured Client. A non-nil
// wrapper is composed around its RoundTripper; nil keeps the configured base
// transport unchanged.
func NewClientFromOptionsWithTransport(ctx context.Context, options *Options, wrapper TransportWrapper) (*Client, error) {
	clientConfig, err := BuildClientConfig(ctx, options, TransportConfig{})
	if err != nil {
		return nil, err
	}
	if wrapper != nil {
		clientConfig.RoundTripper = wrapper(clientConfig.RoundTripper)
	}
	return NewClientFromClientConfig(clientConfig), nil
}

// NewClientFromClientConfig builds a Client from an already assembled runtime
// configuration without replacing or rewrapping its RoundTripper.
func NewClientFromClientConfig(cfg *ClientConfig) *Client {
	return &Client{
		RoundTripper: cfg.RoundTripper,
		Server:       cfg.Server,
		// set default response handler
		OnResponse: StatusOnResponse,
	}
}

func NewClient(server string) (*Client, error) {
	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	return NewClientFromClientConfig(&ClientConfig{Server: serverURL}), nil
}

func (c *Client) Head(path string) *Builder {
	return c.Request(http.MethodHead, path)
}

func (c *Client) Get(path string) *Builder {
	return c.Request(http.MethodGet, path)
}

func (c *Client) Post(path string) *Builder {
	return c.Request(http.MethodPost, path)
}

func (c *Client) Put(path string) *Builder {
	return c.Request(http.MethodPut, path)
}

func (c *Client) Patch(path string) *Builder {
	return c.Request(http.MethodPatch, path)
}

func (c *Client) Delete(path string) *Builder {
	return c.Request(http.MethodDelete, path)
}

func (c *Client) Request(method string, path string) *Builder {
	return NewRequest(method, path).
		OnRequest(c.OnRequest).
		OnResponse(c.OnResponse).
		Client(c.Client).
		RoundTripper(c.RoundTripper).
		BaseAddr(c.Server).
		Debug(c.Debug)
}
