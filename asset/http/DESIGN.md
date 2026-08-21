# HTTP adapter design

This package owns both sides of the Asset HTTP protocol. `Server` depends only
on `asset.Service`; `Client` implements `asset.Service`. Client construction
uses one adapter-owned Options value for serializable configuration. Runtime
transport composition remains an explicit `NewWithTransport` argument rather
than becoming part of Options.

Management and content routes are separate Groups so a host controls their
prefix and access policy. `Resolve` is one content request. The response is
either a redirect represented as `asset.Link` or a readable response body.
Both forms carry the complete Asset descriptor in `X-Asset-Descriptor`, so one
Resolve request returns the same domain result as a local implementation.
The Server owns this response projection and delegates only generic unresolved
or pre-resolved byte serving to `rest/api`.
Readable Blob content uses the request body. Link-backed Blob input uses the
dedicated `application/vnd.xiaoshiai.asset-link+json` representation, keeping
ordinary JSON files unambiguous.
OAuth audiences, scopes, credentials, and service discovery are outside this
package.
