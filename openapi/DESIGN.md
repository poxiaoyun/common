# Design

The plugin owns OpenAPI document construction and route projection. Callers
own service titles and select authentication mechanisms from their runtime
configuration.

Path templates are projections of matcher-owned patterns. Projection preserves
capture names and real trailing slashes while omitting regular expressions,
multi-segment markers, and end markers. It never changes a registered route's
pattern. Anonymous subtrees do not introduce documentation parameters.

Routes sharing an HTTP method and path describe one OpenAPI operation. Media
variants contribute request-body content and response content keyed by media
type; response status codes form a union. Operation metadata, request-body
requirements, and metadata of responses with the same status must agree.
Conflicting definitions of the same media type are construction errors rather
than replacements. Comparison uses the serialized OpenAPI contract, so omitted
empty collections and resolved reference caches do not create false conflicts.
An unsuccessful merge leaves the previously documented operation unchanged.

The plugin initializes its route prefix to `/openapi`. Services that need a
different mount point override the plugin path during construction.

Document customization is an in-place construction-time operation:
`ConfigureDocument` returns the same plugin so callers can chain configuration
without an empty-path sentinel or a separate options accumulator.
Path customization follows the same construction-time chaining model through
`WithPath`.

The package exposes the document as the `Document` alias so callers compose
configuration through `common/openapi` without importing the underlying
OpenAPI implementation package.

`ConfigureAuthenticationSecurity` translates a service's effective runtime
authentication mechanisms into component security schemes and document-level
alternatives. Each credential mechanism occupies a separate security
requirement because OpenAPI combines requirements as alternatives and combines
schemes within one requirement as all-of. Anonymous access is represented by
an empty security requirement.

OpenID Connect accepts the provider issuer and owns the protocol projection to
its well-known discovery endpoint. OAuth 2.0 authorization-code and
client-credentials flows accept domain-level endpoint names and are translated
to the OpenAPI wire fields only inside this package. Proxy Header and OAuth 2.0
flow options reserve those standard document shapes for services whose runtime
authentication configuration actually supplies them; the OpenAPI package does
not enable authentication mechanisms.
