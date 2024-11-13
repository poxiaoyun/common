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

func (c *Client) Delete(path string) *Builder {
	return c.Request(http.MethodDelete, path)
}

func (c *Client) Request(method string, path string) *Builder {
	return NewRequest(method, path).
		OnRequest(c.OnRequest).
		OnResponse(c.OnResponse).
		Client(c.Client).
		RoundTripper(c.RoundTripper).
		BaseAddr(c.Server)
}

func GetWebSocket(ctx context.Context, cliconfig *ClientConfig, reqpath string, queries url.Values, onmsg func(ctx context.Context, msg []byte) error) error {
	log := log.FromContext(ctx).WithValues("path", reqpath, "queries", queries)
	u, err := MergeURL(cliconfig.Server, reqpath, queries)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	dailer := websocket.Dialer{
		NetDialContext: cliconfig.DialContext,
	}
	if cliconfig.RoundTripper != nil {
		if httptransport, ok := cliconfig.RoundTripper.(*http.Transport); ok {
			dailer.TLSClientConfig = httptransport.TLSClientConfig
		}
	}
	log.V(5).Info("common http client websocket", "url", u.String())
	wsconn, resp, err := dailer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	go func() {
		// keep alive
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				if err := wsconn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

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
			case websocket.CloseMessage:
				return nil
			case websocket.TextMessage, websocket.BinaryMessage:
				if err := onmsg(ctx, message); err != nil {
					return err
				}
			}
		}
	}
}
