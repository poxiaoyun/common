# REST API design

## Authentication seam

Authentication converts transport credentials into one canonical `AuthenticationInfo` value. `Subject` is the identity the request is about. An optional `Actor` is the current identity acting for that subject. An optional `Access` contains audience and scope constraints carried by an OAuth 2.0 access token. Groups belong to a subject; audiences and scopes belong to the credential used for this request. Protocol claims and provider-specific metadata do not flow through an unstructured attribute map.

Context helpers are named for the value they carry: `WithAuthentication` pairs with `AuthenticationFromContext`, and `WithAuthorizationDecision` pairs with `AuthorizationDecisionFromContext`. Authentication operations retain the verb `Authenticate`; values and filters use the noun `Authentication`.

`Subject.ID` is the stable authorization and audit identifier. `Name` and `Email` are display attributes and must not be used as authorization keys. An OIDC adapter must resolve the issuer and `sub` pair into the authentication domain's stable subject ID. It must not infer identity semantics from provider-specific ID prefixes or from comparisons with `client_id`.

At the request seam, an authenticator returns `ErrNotProvided` only when the request does not contain a credential applicable to that authenticator. Credential-level adapters may return `ErrNotProvided` so another adapter can inspect the same credential, but the request adapter that established credential presence converts final non-recognition into an authentication error.

An authentication or authorization adapter that requires an HTTP authentication challenge returns `AuthenticationChallengeError` with the public response status and challenge value. The adapter logs any diagnostic provider error before translating it. The challenge travels through authenticator and authorizer composition without writing response state. The final HTTP error writer is the sole owner that copies the challenge to `WWW-Authenticate` when the request is rejected; generic response handling must not depend on provider packages.

Authentication failures separate public response data from diagnostic errors. An explicit `errors.Status` or challenge is public response data. For any other error, the default authentication filter logs its details from the request context and returns a generic Unauthorized status. A custom `AuthenticationErrorHandlerFunc` owns both logging and response redaction.

`AuthenticatorChain` preserves these ordered-composition invariants:

- `ErrNotProvided` means no applicable credential was supplied to that authenticator. The chain may continue to every later authenticator.
- A recognized but rejected credential records an authentication failure. Later authenticators still run, so another configured protocol may recognize and accept the credential.
- If no authenticator succeeds, the chain returns its recorded failures, or `ErrNotProvided` when every authenticator abstained.

`FallbackAuthenticator` invokes its fallback only when the primary authenticator returns `ErrNotProvided`. Therefore an explicitly configured anonymous fallback applies only to requests without credentials; supplied invalid credentials produce HTTP 401.

`StaticTokenAuthenticator` compares one opaque token and returns one fixed authentication value. It stores only a SHA-256 digest, copies mutable authentication fields, and compares digests in constant time. Deployment-specific subjects, groups, and authorization policy remain with the caller.

`AuthenticationInfo.Clone` is an ownership operation for implementations that retain authentication across requests. Static-token and authentication-cache implementations use it before returning retained values so request-local mutation cannot alter future authentication results. Ordinary request propagation, context storage, audit, and decoded webhook or header values do not clone.

## OAuth resource server seam

An OAuth resource server accepts access tokens, not OIDC ID tokens. The JWT access-token adapter follows the RFC 9068 profile and keeps its protocol claims separate from `AuthenticationInfo`. A client-credentials token maps its verified `sub` to the subject. A delegated token maps the top-level `sub` to the subject and the outermost RFC 8693 `act` claim to the current actor. The OAuth `client_id` remains protocol information and is not used to classify a subject or infer an actor.

`Access` is non-nil only for an OAuth access token, including a token with no scopes. Audience validation happens during token verification. Scopes are authorization information enforced by the resource server. `OAuth2ScopeAuthorizer` handles access-token requests and returns NoOpinion for other authentication modes, allowing callers to use it as one complete policy in an authorization chain. A missing required scope returns a challenged denial that the authorization filter renders as HTTP 403 with the RFC 6750 `insufficient_scope` challenge. The access-token adapter translates a provider's invalid-token error into a challenged authentication error that the authentication filter renders as HTTP 401 with `invalid_token`.

## Authorization seam

`Authorizer` consumes the complete `AuthenticationInfo` and request `Attributes`. This preserves subject, actor, groups, audiences, and scopes through the authorization decision. Business authorizers use stable subject IDs; policies that care about delegation inspect the current actor explicitly.

`Decision` expresses only Allow, Deny, or NoOpinion. The accompanying reason is human-readable and never controls the HTTP response. An Authorizer that intentionally requires a specific denial response returns a structured `errors.Status`; the authorization filter preserves it, while ordinary evaluation errors become Forbidden. This allows resource-hiding policies to return NotFound without a magic reason string.

Authorization reasons and explicit `errors.Status` messages are public response data. Any other authorization error is diagnostic: the filter logs its details from the request context and returns only a generic Forbidden status. This avoids exposing policy, storage, or provider details without introducing another error wrapper.

`AuthorizerChain` composes alternative complete policies: Allow and Deny are final, while NoOpinion continues. A scope policy may participate when it completely decides access-token requests and later policies handle other request kinds. When a deployment requires scope and a local policy for the same request, it must provide an explicit all-of Authorizer instead of relying on the first-decisive chain. `NewAuthorizationFilter` enforces one already-composed Authorizer and does not choose policy-combining semantics.

`AuthorizationFilter` adapts the domain `Authorizer` seam to `RequestAuthorizer`, then applies the shared HTTP decision flow as a `Filter`. Its `RequestAuthorizer` implementation is intentional so domain and transport authorizers use identical prior-decision and response semantics.

## Trace seam

Domain filters do not depend on OpenTelemetry or mutate spans. `trace.go` owns HTTP tracing and optional request enrichment. `NewEndUserTraceFilter` consumes the authentication context after `NewAuthenticationFilter`; `NewAuthorizationTraceFilter` consumes request attributes after `NewAttributeExtractionFilter`. Both are explicit composition choices because end-user identifiers and authorization resource names may be sensitive or high-cardinality. Default route tracing records the route template but not dynamic path-variable values.

## Trusted propagation

Authentication webhooks and trusted request-header propagation carry the complete `AuthenticationInfo` structure. Request headers use one encoded authentication envelope so additions to the canonical value cannot be silently dropped by parallel field lists. The envelope is an assertion, not a credential; inbound use requires a `RequestHeaderTrustVerifier`, and the trusted proxy must remove client-supplied assertion headers.

## List pagination seam

`meta.ListOptions` and `meta.Page` are the shared list transport contract.
`size` is the only batch-length term. `continue` is opaque and takes precedence
over `page`; a positive page selects page pagination. A request with neither is
mode-neutral, so the owning service selects its default without changing the
shared parser. Collection `resourceVersion` is separate from item versions and
is omitted when the backend cannot express it as the shared integer contract.
`GetListOptions` accepts boundary-owned defaults for size and sort. Defaults
are applied before request fields, so an explicitly provided zero size or empty
sort remains explicit. They never modify Page or Continue.
