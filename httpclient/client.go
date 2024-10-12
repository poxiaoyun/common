package httpclient

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"xiaoshiai.cn/common/log"
)

type Client struct {
	Client       *http.Client
	RoundTripper http.RoundTripper
	Server       string
	OnRequest    func(req *http.Request) error
	OnResponse   func(req *http.Request, resp *http.Response) error
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

func (c *Client) GetWebSocket(ctx context.Context, reqpath string, queries url.Values, onmsg func(ctx context.Context, msg []byte) error) error {
	u, err := MergeURL(c.Server, reqpath, queries)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	dailer := websocket.Dialer{}
	if c.RoundTripper == nil {
		c.RoundTripper = http.DefaultTransport
	}
	if httptransport, ok := c.RoundTripper.(*http.Transport); ok {
		dailer.TLSClientConfig = httptransport.TLSClientConfig
	}
	log.FromContext(ctx).V(5).Info("common http client websocket", "url", u.String())
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
