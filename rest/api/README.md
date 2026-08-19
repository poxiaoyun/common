# REST API

`rest/api` provides the HTTP routing, authentication, authorization, audit, and request-context interfaces shared by services using `common`.

Authenticators normalize credentials into `AuthenticationInfo`, which embeds the request `Subject` and may include a current `Actor` and OAuth access constraints. `Subject.ID` is the stable key for authorization, ownership, and audit. Display names are not identity keys.

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

`StaticTokenAuthenticator` maps one opaque token to a fixed `AuthenticationInfo`. Request-header and webhook adapters transport the same canonical value without provider-specific attribute maps.

`AuthenticationReview` and `AuthorizationReview` are shared wire contracts.
The webhook authenticator, authorizer, and audit sink can receive a
`WebhookTransportWrapper`, allowing a service to apply one OAuth Client
Credentials transport to all calls to an IAM Resource Server while retaining
each endpoint's own timeout, proxy, and TLS settings. Token authentication
reviews may request audiences; the response must contain at least one validated
requested audience. Basic and SSH reviews are audience-unaware.

Resource List APIs use `meta.Page[T]` and `meta.ListOptions`. The query field
for batch length is `size`; `limit` is not a second spelling. A non-empty
`continue` selects continuation pagination and takes precedence over `page`.
Defaults and maximum sizes belong to the service that owns the HTTP boundary.
Services can pass `meta.DefaultSize` and `meta.DefaultSort` to `GetListOptions`; these
options only fill omitted values and do not choose or rewrite the pagination
mode.
