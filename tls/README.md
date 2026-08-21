# TLS configuration

`tls` builds standard-library client and server TLS configurations from files.

Use `NewClientConfig` for outbound connections. With no CA file it leaves
`RootCAs` unset so Go uses system roots. With a CA file it adds that bundle to
the system roots. A client certificate and key must be supplied together.

Use `NewServingConfig` for inbound TLS. It requires a certificate and key.

Certificate pairs are loaded and validated during construction. New TLS
handshakes re-read them from disk at most once per minute. A reload failure is
returned by the handshake and a later handshake can recover after the files are
corrected. Existing connections are unaffected. CA bundles are fixed for the
lifetime of a configuration.

Neither constructor starts goroutines or accepts a context; the owner of the
HTTP request or server supplies context for its own runtime lifecycle.
