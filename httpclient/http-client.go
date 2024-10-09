package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
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

func (c *Client) fullurl(reqpath string, queries url.Values) (*url.URL, error) {
	u, err := url.Parse(c.Server)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, reqpath)
	existsQuery := u.Query()
	for k, v := range queries {
		existsQuery[k] = v
	}
	u.RawQuery = existsQuery.Encode()
	return u, nil
}

func (c *Client) GetWebSocket(ctx context.Context, reqpath string, queries url.Values, onmsg func(ctx context.Context, msg []byte) error) error {
	u, err := c.fullurl(reqpath, queries)
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

func (c *Client) Get(ctx context.Context, path string, queries url.Values, decodeinto any) error {
	_, err := c.Do(ctx, Get(path).Queries(queries).Return(decodeinto))
	return err
}

func (c *Client) Post(ctx context.Context, path string, data any) error {
	_, err := c.Do(ctx, Post(path).Body(data))
	return err
}

func (c *Client) Put(ctx context.Context, path string, queries url.Values, data any) error {
	_, err := c.Do(ctx, NewRequest(http.MethodPut, path).Queries(queries).Body(data))
	return err
}

func (c *Client) Delete(ctx context.Context, path string) error {
	_, err := c.Do(ctx, NewRequest(http.MethodDelete, path))
	return err
}

type RequestBuilder struct {
	method   string
	path     string
	queries  url.Values
	headers  http.Header
	body     any
	decodeto any
}

func NewRequest(method string, path string) *RequestBuilder {
	return &RequestBuilder{method: method, path: path}
}

func Get(path string) *RequestBuilder {
	return NewRequest(http.MethodGet, path)
}

func Post(path string) *RequestBuilder {
	return NewRequest(http.MethodPost, path)
}

func (r *RequestBuilder) Query(key, value string) *RequestBuilder {
	if r.queries == nil {
		r.queries = url.Values{}
	}
	r.queries.Add(key, value)
	return r
}

func (r *RequestBuilder) Queries(queries url.Values) *RequestBuilder {
	if r.queries == nil {
		r.queries = url.Values{}
	}
	for k, v := range queries {
		r.queries[k] = v
	}
	return r
}

func (r *RequestBuilder) Header(key, value string) *RequestBuilder {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	r.headers.Add(key, value)
	return r
}

func (r *RequestBuilder) Headers(headers http.Header) *RequestBuilder {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	for k, v := range headers {
		r.headers[k] = v
	}
	return r
}

func (r *RequestBuilder) Body(data any) *RequestBuilder {
	r.body = data
	return r
}

func (r *RequestBuilder) Return(decodeinto any) *RequestBuilder {
	r.decodeto = decodeinto
	return r
}

func (c *Client) Do(ctx context.Context, r *RequestBuilder) (*http.Response, error) {
	resp, err := c.DoRaw(ctx, r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusIMUsed {
		bytes, _ := io.ReadAll(resp.Body)
		return nil, errors.New(string(bytes))
	}
	switch into := r.decodeto.(type) {
	case io.Writer:
		_, err := io.Copy(into, resp.Body)
		return nil, err
	case nil:
	default:
		return resp, json.NewDecoder(resp.Body).Decode(r.decodeto)
	}
	return resp, nil
}

func (c *Client) DoRaw(ctx context.Context, r *RequestBuilder) (*http.Response, error) {
	contenttype := r.headers.Get("Content-Type")
	var body io.Reader
	switch typed := r.body.(type) {
	case io.Reader:
		body = typed
	case []byte:
		body = bytes.NewReader(typed)
	case string:
		body = bytes.NewBufferString(typed)
	case nil:
	default:
		bts, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(bts)
		if contenttype == "" {
			contenttype = "application/json"
		}
	}

	u, err := c.fullurl(r.path, r.queries)
	if err != nil {
		return nil, err
	}

	log.FromContext(ctx).V(5).Info("common http client request", "method", r.method, "url", u.String())
	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contenttype != "" {
		req.Header.Set("Content-Type", contenttype)
	}
	// set headers
	for k, v := range r.headers {
		req.Header[k] = v
	}
	if c.OnRequest != nil {
		if err := c.OnRequest(req); err != nil {
			return nil, err
		}
	}
	// init client if not set
	if c.Client == nil {
		if c.RoundTripper == nil {
			c.Client = http.DefaultClient
		} else {
			c.Client = &http.Client{Transport: c.RoundTripper}
		}
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if c.OnResponse != nil {
		if err := c.OnResponse(req, resp); err != nil {
			return resp, err
		}
	}

	return resp, nil
}
