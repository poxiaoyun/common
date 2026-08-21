# HTTP client design

## Transport construction

`BuildClientConfig` owns the default HTTP transport, TLS, proxy, and
configured authentication layers. Its runtime `TransportConfig` configures
that transport without becoming part of the declarative Options. In particular,
DialContext is installed on the default `*http.Transport` before TLS, proxy,
and authentication are composed.

Construction is synchronous and has no lifetime context. Custom roots are read
once while building the transport. A configured client certificate and key are
validated during construction, then re-read through the TLS handshake callback
at most once per minute. This keeps credential rotation on the connection
lifecycle and gives the client no background watcher or Close responsibility.

`RequestAuthenticator` is the protocol-independent authentication seam. An
authentication transport implements it in addition to `http.RoundTripper` so
the same credential policy can prepare ordinary HTTP requests and WebSocket
opening handshakes. The adapter owns Authorization precedence and token
refresh; protocol clients only provide a request with its final target and
context. Wrapped transport discovery selects the outermost authenticator, so a
caller-supplied identity overrides authentication configured on the base
transport in the same way for HTTP and WebSocket requests.

Callers that need additional behavior explicitly wrap the returned
`ClientConfig.RoundTripper` before passing it to `NewClientFromClientConfig`.
`NewClientFromOptionsWithTransport` is the dedicated convenience constructor
for that sequence and treats a nil wrapper as no additional layer;
`BuildClientConfig` itself remains wrapper-agnostic.
A complete replacement remains caller-owned and therefore also owns its TLS
and proxy behavior.

`ClientConfig.DialContext` and Proxy are retained for WebSocket and other
non-standard protocol clients. WebSocket obtains TLS through
`TLSClientConfig`. `NewClientFromClientConfig` does not use DialContext to
replace the assembled RoundTripper.

`TLSClientConfigHolder`, `WrappedRoundTripper`, and `TLSClientConfig` expose the
TLS configuration through known and wrapped transport implementations. This
supports protocols that need to inspect the effective TLS settings without
making that inspection part of ordinary HTTP client construction.

## WebSocket client

`WebSocketClient` is the WebSocket seam. Construction resolves the reusable
server, DialContext, TLS, default proxy, and outermost request authenticator
from ClientConfig. `Stream` authenticates a fresh opening-handshake request
after copying the caller's per-stream headers, then owns the connection
lifecycle, keepalive, control frames, and message loop. Its message handler is
a required argument; `WebSocketOptions` contains only per-stream optional
values and cannot create a stream that silently discards messages.
`StreamWebSocket` is the one-off convenience seam for a complete URL. It uses
system TLS, the environment proxy, and the default keepalive interval, while
delegating connection ownership and message consumption to `WebSocketClient`.

## Pagination seam

`ListOptionsToQuery` is a transport projection. It serializes every non-zero
list field without selecting, validating, or normalizing pagination behavior;
the service executing the list owns those semantics.

`ListAll` owns client-side traversal of paginated list responses. Callers supply
one list operation and ordinary `meta.ListOptions`; they do not reproduce page
or continuation-token state machines. `ListAll` trusts both its options and
returned pagination metadata and does not validate them.

The response metadata selects the next request. A positive `Limit` identifies a
continuation response: a non-empty `Continue` advances, while an empty token
ends traversal. Otherwise a positive `Page` advances while `Total` shows more
results. A response with neither is a one-shot result and ends traversal. Empty
continuation batches are valid and must still advance when they carry a token.

`ListAll` returns no partial result after an error. Its aggregate collection
resource version is retained only when every response reports the same positive
version, because a mixed-version result is not one collection snapshot.
