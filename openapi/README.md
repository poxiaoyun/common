# OpenAPI

`openapi` projects `rest/api` routes into an OpenAPI 3.1 document and serves
the document and UI through `NewAPIDocPlugin`.

By default, the UI is available at `/openapi/` and the OpenAPI 3.1 document at
`/openapi/openapi.json`. Chain `WithPath` to mount both endpoints under a
different prefix.

Plugin configuration callbacks receive `*openapi.Document`; callers do not
need to import the underlying OpenAPI implementation package.

Create the default plugin with `NewAPIDocPlugin()`. Services that need to set
document metadata or authentication schemes chain
`ConfigureDocument(func(*Document))` before installing it.

Use `ConfigureAuthenticationSecurity` from the plugin configuration callback
to project the service's effective authentication options. Bearer tokens,
Basic credentials, a session cookie, OpenID Connect, OAuth 2.0
authorization-code or client-credentials flows, and a trusted proxy header are
independent alternatives. Set `Anonymous` when requests without credentials
are accepted.

For OpenID Connect, pass the issuer. The package derives the provider metadata
URL required by OpenAPI. OAuth 2.0 options use authorization, token, and refresh
endpoint terminology; the package owns their translation to OpenAPI wire
fields. Zero-value mechanisms are omitted. These settings describe runtime
behavior but do not enable an authenticator.
