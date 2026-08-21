package httpclient

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"xiaoshiai.cn/common/log"
)

// WebSocketClient streams WebSocket messages using one resolved ClientConfig.
type WebSocketClient struct {
	server        url.URL
	dialer        websocket.Dialer
	authenticator RequestAuthenticator
}

// StreamWebSocket connects to address with the default network configuration
// and consumes messages until the context ends, the peer closes the
// connection, or onMessage returns an error.
func StreamWebSocket(ctx context.Context, address string, onMessage func(context.Context, []byte) error) error {
	server, err := url.Parse(address)
	if err != nil {
		return err
	}
	client, err := NewWebSocketClient(&ClientConfig{
		Server: server,
		Proxy:  http.ProxyFromEnvironment,
	})
	if err != nil {
		return err
	}
	return client.Stream(
		ctx,
		"",
		func(ctx context.Context, _ int, message []byte) error {
			return onMessage(ctx, message)
		},
		WebSocketOptions{KeepAliveInterval: 30 * time.Second},
	)
}

// NewWebSocketClient builds a reusable WebSocket client from ClientConfig.
func NewWebSocketClient(clientConfig *ClientConfig) (*WebSocketClient, error) {
	dialer := websocket.Dialer{
		NetDialContext: clientConfig.DialContext,
		Proxy:          clientConfig.Proxy,
	}
	if clientConfig.RoundTripper != nil {
		tlsConfig, err := TLSClientConfig(clientConfig.RoundTripper)
		if err != nil {
			return nil, err
		}
		if tlsConfig != nil {
			// WebSocket opening handshakes use HTTP/1.1. Clone the transport TLS
			// configuration so selecting the protocol here does not mutate the
			// HTTP client's transport or disable its HTTP/2 support.
			dialer.TLSClientConfig = tlsConfig.Clone()
			dialer.TLSClientConfig.NextProtos = []string{"http/1.1"}
		}
	}
	return &WebSocketClient{
		server:        *clientConfig.Server,
		dialer:        dialer,
		authenticator: FindRequestAuthenticator(clientConfig.RoundTripper),
	}, nil
}

// WebSocketOptions controls one WebSocket stream.
type WebSocketOptions struct {
	// Queries are merged with any query parameters already present in the base
	// server URL and request path.
	Queries url.Values
	// Header is sent in the opening HTTP handshake.
	Header http.Header
	// KeepAliveInterval sends WebSocket ping control messages while the stream is
	// open. Zero disables client-generated pings.
	KeepAliveInterval time.Duration
	// ProxyURL overrides the proxy resolved from ClientConfig for this stream.
	ProxyURL *url.URL
}

// Stream connects and consumes messages until the context ends, the peer
// closes the connection, or onMessage returns an error.
func (c *WebSocketClient) Stream(
	ctx context.Context,
	requestPath string,
	onMessage func(ctx context.Context, kind int, message []byte) error,
	options WebSocketOptions,
) error {
	log := log.FromContext(ctx).WithValues("path", requestPath, "queries", options.Queries)
	u := MergeURL(c.server, requestPath, options.Queries)
	header := options.Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	request.Header = header
	if c.authenticator != nil {
		if err := c.authenticator.AuthenticateRequest(request); err != nil {
			return err
		}
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	dialer := c.dialer
	if options.ProxyURL != nil {
		dialer.Proxy = http.ProxyURL(options.ProxyURL)
	}
	log.V(6).Info("common http client websocket", "url", u.String())
	connection, _, err := dialer.DialContext(ctx, u.String(), request.Header)
	if err != nil {
		return err
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() {
		connection.Close()
	})
	defer stop()
	done := make(chan struct{})
	defer close(done)

	if options.KeepAliveInterval != 0 {
		go func() {
			log.V(3).Info("start keep alive", "interval", options.KeepAliveInterval)
			timer := time.NewTimer(options.KeepAliveInterval)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-timer.C:
					deadline := time.Now().Add(options.KeepAliveInterval)
					if err := connection.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
						log.V(5).Error(err, "failed to send ping")
						return
					}
					timer.Reset(options.KeepAliveInterval)
				}
			}
		}()
	}

	for {
		messageType, message, err := connection.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Error(err, "failed to read message")
			return err
		}
		switch messageType {
		case websocket.TextMessage, websocket.BinaryMessage:
			if err := onMessage(ctx, messageType, message); err != nil {
				return err
			}
		}
	}
}
