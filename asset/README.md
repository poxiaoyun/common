# asset

`asset.Service` is the common caller-facing interface for named binary
attachments. A target is identified by `Target{Kind, Name}` and each
attachment has an asset name. `Asset` describes the stored object but does not
expose a delivery URL; callers use `Resolve` to obtain content or a direct Link.

Choose an implementation at the application composition boundary:

- `asset/store` for a `common/store.Store`;
- `asset/s3` for S3-compatible storage;
- `asset/inmemory` for tests and local processes;
- `asset/http` for a remote service.

The Store, S3, in-memory, and HTTP client types all implement `asset.Service`.
The HTTP server accepts any `asset.Service`, so a service host can combine it
with either local persistence implementation:

```go
assets := assetstore.New(storage, assetstore.Options{})
server := assethttp.NewServer(assets)
```

Hosts that expose Asset content through a domain-specific route can pass a
`Resolved` value to `assethttp.ContentResponse` and serve the resulting
`api.ContentResponse` without duplicating delivery headers or cache rules.

`Put` generates an Asset name when `PutOptions.Name` is empty; otherwise it
creates or replaces that named Asset and advances its content version. A `Blob`
contains exactly one of `Content` or `Link`, a required `ContentType`, and
optional `ContentLength`, `FileName`, and `ModTime` values that are persisted
with the Asset. A zero ContentLength means unknown.
Non-nil `PutOptions.Metadata` stores initial metadata or replaces metadata
during a named Put; nil preserves existing metadata. `ReplaceMetadata` changes
metadata without advancing the content version or changing Digest or ETag.
`Get` returns the complete Asset descriptor, including metadata.
Local service hosts configure upload policies through the selected adapter's
Options; those server-side settings are not part of this caller contract.
`ResolveOptions.Range` carries an RFC 7233 byte-range request. An implementation
may return ranged Content with ContentLength and ContentRange, or return a Link
instead. A Service caller that receives a Link is responsible for sending the
same Range header when fetching that URL; the Range option does not force the
Asset service to proxy the content.

Asset names and target kinds are portable 1-64 character identifiers: they
start and end with an ASCII letter or digit and may contain letters, digits,
periods, underscores, and hyphens. A target name contains one or two such
components of at most 128 characters, separated by one colon for scoped
identities such as `cloud:database`. These values are shared by HTTP paths,
Store identities, and object-storage keys, so path separators, URL
delimiters, whitespace, and special path components are not accepted. The
length limits preserve the existing asset API contract; they are not storage
backend limits.

Applications select an adapter directly at their composition root. Backend
dependencies, credentials, OAuth scopes, and route policy remain composition
concerns rather than part of the `asset.Service` contract.
