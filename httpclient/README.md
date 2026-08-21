# HTTP client

`httpclient` provides request construction and transport helpers shared by Go
clients.

`NewClientFromOptions` builds the default transport with TLS, proxy, and static
authentication. `NewClientFromOptionsWithTransport` additionally wraps the
assembled RoundTripper when its wrapper is non-nil; nil reuses the same base
transport behavior as `NewClientFromOptions`. Lower-level adapters can use
`BuildClientConfig`, explicitly wrap the returned `ClientConfig.RoundTripper`,
and then call `NewClientFromClientConfig`. Use
`TLSClientConfig` when another protocol needs to inspect the effective TLS
configuration through a wrapped transport chain.

Authentication transports that must also authenticate non-HTTP protocol
handshakes implement `RequestAuthenticator`. The built-in Bearer and Basic
transports implement it, and wrapped transports may supply their own dynamic
implementation. WebSocket discovers the outermost authenticator in the
transport chain and applies it to the opening handshake, including token
refresh failures in the stream's returned error.

Pass runtime dialing behavior through
`httpclient.TransportConfig{DialContext: ...}`
when building a ClientConfig. The dialer is installed on the underlying HTTP
transport and remains available to WebSocket callers through ClientConfig.
WebSocket also inherits the configured TLS, proxy, and authentication behavior;
call-specific WebSocketOptions may override its proxy and supply additional
handshake headers. The selected authenticator owns whether an existing
Authorization header is preserved or replaced.

Construct `WebSocketClient` from ClientConfig and call `Stream` with a required
message handler. The client owns connection setup, keepalive, Ping/Pong, and
the read loop; WebSocketOptions contains only call-specific optional settings.
For the common one-off case, `StreamWebSocket` accepts a complete URL and a
required message handler, and uses system TLS, the environment proxy, and the
default keepalive interval.

`ListOptionsToQuery` 只负责把非零 `meta.ListOptions` 字段转换成平铺 query；它不判断
分页组合、不选择分页行为，也不规范化字段值。

Use `ListAll` when a caller needs every item from a resource list. The supplied
list function may return page-style, continuation-style, or one-shot responses;
`ListAll` follows the response metadata until the server reports completion.
Filters, selectors, search, and sort options are preserved across requests.
`ListAll` consumes its options and responses without validation.

If any request fails, `ListAll` returns the error and no partial page. The
returned page contains all items and a `Total` equal to their count; traversal
metadata is cleared.
