package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"xiaoshiai.cn/common/log"
)

type ClientConfig struct {
	Server       url.URL
	RoundTripper http.RoundTripper
	DialContext  func(ctx context.Context, network, addr string) (net.Conn, error)
}

type Client struct {
	Client       *http.Client
	Server       string
	RoundTripper http.RoundTripper
	OnRequest    func(req *http.Request) error
	OnResponse   func(req *http.Request, resp *http.Response) error
	Debug        bool
}

func NewClientFromConfig(cfg *ClientConfig) *Client {
	var transport http.RoundTripper
	if cfg.DialContext != nil {
		transport = &http.Transport{DialContext: cfg.DialContext}
	} else {
		transport = cfg.RoundTripper
	}
	return &Client{RoundTripper: transport, Server: cfg.Server.String()}
}

func NewClient(server string) *Client {
	return &Client{Server: server}
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

func GetWebSocket(ctx context.Context, cliconfig *ClientConfig, reqpath string, queries url.Values, onmsg func(ctx context.Context, msg []byte) error) error {
	return GetWebSocketOptions(ctx, cliconfig, reqpath, WebSocketOptions{
		Queries:           queries,
		KeepAliveInterval: 30 * time.Second,
		OnMessage: func(ctx context.Context, kind int, msg []byte) error {
			return onmsg(ctx, msg)
		},
	})
}

type WebSocketOptions struct {
	Queries           url.Values
	Header            http.Header
	KeepAliveInterval time.Duration
	ProxyURL          *url.URL
	OnMessage         func(ctx context.Context, kind int, msg []byte) error
}

func GetWebSocketOptions(ctx context.Context, cliconfig *ClientConfig, reqpath string, options WebSocketOptions) error {
	log := log.FromContext(ctx).WithValues("path", reqpath, "queries", options.Queries)
	u := MergeURL(cliconfig.Server, reqpath, options.Queries)
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	dailer := websocket.Dialer{
		NetDialContext: cliconfig.DialContext,
	}
	if options.ProxyURL != nil {
		dailer.Proxy = http.ProxyURL(options.ProxyURL)
	}
	if cliconfig.RoundTripper != nil {
		if httptransport, ok := cliconfig.RoundTripper.(*http.Transport); ok {
			dailer.TLSClientConfig = httptransport.TLSClientConfig
		}
	}
	log.V(6).Info("common http client websocket", "url", u.String())
	wsconn, _, err := dailer.DialContext(ctx, u.String(), options.Header)
	if err != nil {
		return err
	}
	defer wsconn.Close()

	if options.KeepAliveInterval != 0 {
		go func() {
			log.V(3).Info("start keep alive", "interval", options.KeepAliveInterval)
			// keep alive
			timer := time.NewTimer(options.KeepAliveInterval)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					if err := wsconn.WriteMessage(websocket.PingMessage, nil); err != nil {
						log.V(5).Error(err, "failed to send ping")
						return
					}
					timer.Reset(options.KeepAliveInterval)
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msgtype, message, err := wsconn.ReadMessage()
			if err != nil {
				log.Error(err, "failed to read message")
				return err
			}
			switch msgtype {
			case websocket.PingMessage:
				if err := wsconn.WriteMessage(websocket.PongMessage, nil); err != nil {
					return err
				}
			case websocket.PongMessage:
			case websocket.TextMessage, websocket.BinaryMessage:
				if options.OnMessage != nil {
					if err := options.OnMessage(ctx, msgtype, message); err != nil {
						return err
					}
				}
			}
		}
	}
}
