# REST API

`rest/api` provides the HTTP routing, authentication, authorization, audit, and request-context interfaces shared by services using `common`.

`ContentResponse` represents local content or a redirect. Local content with a
`Content-Range` header must already contain the selected bytes and is written as
HTTP 206; other local content is passed to `ServeContent` for conditional and
range request handling. Call `ServePartialContent` directly for already-selected
bytes. It preserves existing `Content-Range` and `Content-Length` headers and
uses the corresponding arguments only when those headers are absent. Pass zero
content length to omit the generated length header. The reader is always copied
to EOF without validating its bytes against the declared length.

Authenticators normalize credentials into `AuthenticationInfo`, which embeds the request `Subject` and may include a current `Actor` and OAuth access constraints. `Subject.ID` is the stable key for authorization, ownership, and audit; `Subject.Name` is the provider-verified username or principal name; `Subject.DisplayName` is a non-unique human-facing label.

Callers compose authenticators and install the result through `NewAuthenticationFilter`. `FallbackAuthenticator` adds an explicit fallback around a completed request authenticator; use `NewFallbackAuthenticator(chain, NewAnonymousAuthenticator())` when requests without credentials should receive the anonymous subject. An invalid supplied credential is never downgraded to anonymous.

`AuthenticationChallengeError` carries a public response status and `WWW-Authenticate` value through authenticator and authorizer composition. Provider adapters log diagnostic errors before translating them into this shared response error. The final HTTP error writer writes the challenge only after the request is rejected. `NewBearerTokenAuthenticationFilter` returns a bare `Bearer` challenge when no more specific challenge is present. Invalid OAuth access tokens add `error="invalid_token"`, while insufficient scope produces HTTP 403 with `error="insufficient_scope"`.

Authorizers receive the complete `AuthenticationInfo`. OAuth scopes are access-token authorization rules. `OAuth2ScopeAuthorizer{}` parses the default `<action>:<resource>` convention and matches each granted scope against request `Attributes`. Arbitrary actions such as `create` or `publish` match exactly; `read` covers `get` and `list`, while `write` covers other actions. `NewOAuth2ScopeMatcher` composes a different aggregate-action matcher or logical-resource matcher without reversing the authorization flow into request-to-scope generation. The default resource matcher uses only the final request resource, so parent resources do not authorize nested targets. `OAuth2ScopeAuthorizer` may be composed with other complete, alternative policies through `AuthorizerChain`; deployments that require both scopes and local policy must provide an Authorizer with those explicit combining semantics before installing `NewAuthorizationFilter`.

Wrap a route extractor with `ServiceAttributesExtractor("cloud", extractor)`
when authorization and audit policy must identify the target Resource Server.
The wrapper sets `Attributes.Service`; the wrapped extractor continues to own
action and resource parsing.

Authorization reasons are descriptive only. To intentionally return a specific denial status, such as hiding a resource with HTTP 404, an Authorizer returns the corresponding `common/errors.Status`; untyped evaluation errors are returned as HTTP 403.

Authentication and authorization errors are diagnostic by default: filters record their details through the logger in the request context and return generic 401 or 403 responses. Reasons and explicit `common/errors.Status` messages are considered intentionally public and must not contain secrets.

Authentication and authorization filters do not write trace data. OpenTelemetry behavior is owned by `trace.go` and composed explicitly: install `NewEndUserTraceFilter` after authentication and `NewAuthorizationTraceFilter` after request attributes only when those potentially sensitive or high-cardinality attributes are required. Route tracing records the low-cardinality `http.route` template and does not record dynamic path-variable values.

`StaticTokenAuthenticator` maps one opaque token to a fixed `AuthenticationInfo`. Request-header and webhook adapters transport the same canonical value without provider-specific attribute maps. Trusted request-header propagation uses one multi-header representation based on the Kubernetes authenticating-proxy convention: `X-Remote-User`, `X-Remote-Uid`, and repeated `X-Remote-Group` carry the standard fields, while display name, email, Actor, Access, audiences, and scopes use fixed `X-Remote-Extra-*` extension fields. `X-Remote-Extra-Access: oauth2` distinguishes a non-nil empty OAuth access constraint from authentication without an access token.

Proxy transports clone the outbound request and remove all configured authentication headers before either injecting fixed/context authentication or forwarding a route without authentication. This prevents client-supplied assertions from crossing either protected or public proxy routes.

`AuthenticationReview` and `AuthorizationReview` are shared wire contracts.
The webhook authenticator, authorizer, and audit sink can receive an
`httpclient.TransportWrapper`, allowing a service to apply one OAuth Client
Credentials transport to all calls to an IAM Resource Server while retaining
each endpoint's own timeout, proxy, and TLS settings. Token authentication
reviews may request audiences; the response must contain at least one validated
requested audience. Basic and SSH reviews are audience-unaware.

`FanoutAuditSink` delivers an immutable event to every configured audit sink in
parallel and aggregates their errors after all sinks have been attempted.
Services that use best-effort asynchronous audit delivery should wrap each
destination in its own `CachedAuditSink` before composing the fan-out so each
destination has independent backpressure.

Resource List APIs use `meta.Page[T]` and `meta.ListOptions`; the authoritative
field contract is documented by [meta](../../meta/README.md#列表与分页). Query fields are
flat and have no mode discriminator. A positive `limit` selects continuation
pagination and `continue` is its optional opaque cursor. Otherwise a positive
`size` selects page pagination; `page` values below one are treated as one.
When neither `limit` nor `size` is positive, the owning service uses its
unpaginated behavior. Fields outside the selected behavior are ignored.
Services can pass
`meta.DefaultPage`, `meta.DefaultContinuation`, and `meta.DefaultSort` to
`GetListOptions`. Default options only write their owned fields. `DefaultPage`
does not fill page fields when a non-empty `continue` or positive `limit`
expresses continuation intent, and `DefaultContinuation` does not fill `limit`
when a non-zero `page` or positive `size` expresses page intent. Query values are parsed first and options are then
applied in declaration order. Responses include only
`page/size/total`, `continue/limit`, or `total`,
according to the selected behavior. A continuation response always retains
`limit`; an omitted or empty `continue` means traversal is complete.

| Canonical query values | Result |
| --- | --- |
| Positive `limit`, optional `continue` | Continuation pagination |
| Otherwise, positive `size` | Page pagination; `page < 1` becomes `1` |
| Otherwise | Unpaginated service behavior |

The request and list-options helpers use the same selection table and ignore
fields outside the selected behavior. For continuation, the independent
`getID` projection must return a stable, unique, non-empty value; the name
projection remains dedicated to search and sorting. Object helpers use the UID
exposed by Store or Kubernetes objects. The helper uses the UID as the opaque
cursor after filtering and sorting.
A cursor missing from the current list returns ResourceExpired. This in-memory
traversal does not guarantee a stable snapshot across requests.

Use `PageFromPreparedList` when the input is already filtered and sorted and
only page/size slicing remains. It preserves the same `page < 1` and `size < 0`
normalization described above.
