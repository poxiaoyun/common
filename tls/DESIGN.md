# TLS configuration design

## Ownership and lifecycle

The package owns translating certificate files into `crypto/tls.Config`.
Callers own connection and server lifetimes; construction therefore has no
context, background watcher, or Close operation.

Custom trust roots are construction-time policy. When configured, the CA file
is appended once to the system pool. Without a custom CA, `RootCAs` stays nil
and `crypto/tls` selects system roots.

Certificate identity follows the TLS handshake lifecycle. Client and serving
configurations install the appropriate standard-library callback over one
shared loader. The loader validates the initial pair, serializes access, and
caches one load result for one minute. After that interval, the next handshake
replaces the cached result by reading both files again. A failed read or parse
is returned to that handshake rather than retaining an identity that no longer
matches the files; a later refresh can recover.

Existing connections retain the identity negotiated by TLS. Rotation only
affects new handshakes, matching the boundary at which a certificate is used.
