# Asset design

## Responsibility

`asset.Service` is the module boundary. It describes the operations an asset
caller needs: put, inspect, list, replace metadata, resolve content, and delete.
`Put` generates an Asset name when omitted and replaces the named Asset when
provided. Its Blob contains either readable content or a direct Link together
with required media type and optional declared length and source-file metadata.
Zero declared length means unknown. Content implementations verify a non-zero
declaration against the bytes consumed; a retained Link uses it as the Asset
size. `PutOptions.Metadata` supplies initial metadata or replaces existing
metadata when non-nil; nil preserves metadata during named content replacement.
`ReplaceMetadata` replaces caller-defined metadata without replacing content,
advancing the content Version, or changing Digest or ETag. `Get` already returns
the complete Asset descriptor, so there is no separate metadata read operation.
The contract does not expose persistence records, backend selection, HTTP
authentication, or deployment configuration.

The root package owns the stable domain and wire types and identity rules.
Reading, policy enforcement, digesting, sorting, pagination, persistence, and
HTTP response projection remain adapter implementation details.
The Store implementation delegates filtering, sorting, and pagination to
`common/store` and preserves the Store's page metadata.

## Implementations

Implementations sit behind `asset.Service` in behavior-specific subpackages:

- `asset/store` persists metadata and content in a `common/store.Store`;
- `asset/s3` persists each asset as an S3 object;
- `asset/inmemory` supports tests and local processes;
- `asset/http.Client` accesses a remote asset service.

`asset/http.Server` depends only on `asset.Service`. An IAM process can mount
that server over either the Store or S3 implementation without changing the
HTTP contract. A caller can replace the local implementation with the HTTP
client without changing its asset usage.

Implementation selection belongs at the composition boundary. A host directly
constructs `asset/store`, `asset/s3`, or `asset/http` with that adapter's own
dependencies and Options. The module does not add a universal factory whose
configuration would merely duplicate those adapter interfaces.

Upload-policy, connection, and location settings are server-side concerns.
Each local adapter owns its own Options; the caller-facing root package does
not expose or abstract implementation configuration. Every local adapter uses
the root package's media-type matcher for `AllowedMediaTypes`: an empty list
accepts every media type, an exact entry accepts that media type, and a
wildcard media range such as `image/*` accepts every subtype. Matching ignores
media-type parameters.

## HTTP projection

The HTTP package owns both sides of one protocol. Management and content
delivery are separate Groups so the host decides their route prefix,
authentication, authorization, and public visibility. OAuth audiences, scopes,
credentials, and discovery remain host concerns.

`Blob.Content` and `Blob.Link` are mutually exclusive. A stored direct Link is
returned directly by `Service.Resolve`; materialized content may be returned as
content or as an adapter-generated direct Link. `Link` is a self-contained URL
and carries no request or response headers. An implementation may read and
materialize an input Link during Put, or retain it as the Asset's direct
delivery location. That choice is not visible through the Service contract.

The HTTP client sends readable content as the request body. It sends a Link as
`application/vnd.xiaoshiai.asset-link+json`, so an ordinary JSON asset remains
ordinary content. POST selects generated-name Put semantics and named PUT
selects create-or-replace semantics. `Content-Disposition` carries FileName;
`X-Asset-Mod-Time` carries the source modification time. Neither value is used
as the Asset identity or storage key. `X-Asset-Metadata` carries base64url-encoded
JSON so Put can distinguish omitted metadata from an explicitly empty map.
Content and redirect responses carry the complete Asset descriptor as
base64url-encoded JSON in `X-Asset-Descriptor`, allowing HTTP Resolve to return
the same Asset value as local implementations without an additional request.

`Service.Resolve` returns the `Asset` and exactly one delivery method: a direct
`Link` or readable `Content`. The HTTP server converts that result directly to
a redirect or content response. Content responses preserve conditional and
range request semantics. An Asset does not expose a URL because a caller cannot
infer whether the implementation retained the input Link or materialized its
own copy.
Redirects and presigned URLs are Resolve results, not persistent identifiers.
Range is a Resolve request, not a storage choice. Store and in-memory
implementations may return selected Content, S3 may pass the Range to object
storage, and an implementation may still return a Link. A redirect preserves
the original Range request without requiring the Asset service to materialize
the linked content.

## Persistence compatibility

`asset/store` uses Store resource `assets` scoped by `kind` and `owner`.
`asset/s3` stores objects below `<prefix>/<kind>/<owner>/<asset>` and retains the
existing `asset-*` metadata keys. These names are persistence contracts, so a
host can move an existing deployment to this module without migrating data.
