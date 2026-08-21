# REST API design

## Server TLS lifecycle

`ServeTLS` installs file-backed serving credentials before listening. The
initial certificate pair must load successfully; server option errors stop
startup instead of falling through to a plaintext listener. The TLS package
reloads the pair lazily for new handshakes, so the server runtime context owns
only listener shutdown and no certificate watcher.

## Content delivery seam

`ContentResponse` uses `Content-Range` to identify content whose byte range
has already been resolved by its source. `ServeContentResponse` writes that
content directly as HTTP 206 without applying the request Range again. Content
without `Content-Range` continues through `ServeContent`, which owns conditional
and range request evaluation. `ServePartialContent` is the reusable seam for
already-resolved bytes. Existing `Content-Range` and `Content-Length` response
headers take precedence; its arguments supply either value when the header has
not already been set. Content is copied to EOF without validation against the
declared length. A zero content-length argument generates no length header.

## Authentication seam

Authentication converts transport credentials into one canonical `AuthenticationInfo` value. `Subject` is the identity the request is about. An optional `Actor` is the current identity acting for that subject. An optional `Access` contains audience and scope constraints carried by an OAuth 2.0 access token. Groups belong to a subject; audiences and scopes belong to the credential used for this request. Protocol claims and provider-specific metadata do not flow through an unstructured attribute map.

Context helpers are named for the value they carry: `WithAuthentication` pairs with `AuthenticationFromContext`, and `WithAuthorizationDecision` pairs with `AuthorizationDecisionFromContext`. Authentication operations retain the verb `Authenticate`; values and filters use the noun `Authentication`.

`Subject.ID` is the stable authorization and audit identifier. `Name` is the provider-verified username or principal name within the authentication domain, while `DisplayName` is a non-unique human-facing label. Neither is a stable ownership key. An OIDC adapter must resolve the issuer and `sub` pair into the authentication domain's stable subject ID, map `preferred_username` to `Name`, and map the OIDC `name` claim to `DisplayName`. It must not infer identity semantics from provider-specific ID prefixes or from comparisons with `client_id`.

At the request seam, an authenticator returns `ErrNotProvided` only when the request does not contain a credential applicable to that authenticator. Credential-level adapters may return `ErrNotProvided` so another adapter can inspect the same credential, but the request adapter that established credential presence converts final non-recognition into an authentication error.

An authentication or authorization adapter that requires an HTTP authentication challenge returns `AuthenticationChallengeError` with the public response status and challenge value. The adapter logs any diagnostic provider error before translating it. The challenge travels through authenticator and authorizer composition without writing response state. The final HTTP error writer is the sole owner that copies the challenge to `WWW-Authenticate` when the request is rejected; generic response handling must not depend on provider packages.

Authentication failures separate public response data from diagnostic errors. An explicit `errors.Status` or challenge is public response data. For any other error, the default authentication filter logs its details from the request context and returns a generic Unauthorized status. A custom `AuthenticationErrorHandlerFunc` owns both logging and response redaction.

`AuthenticatorChain` preserves these ordered-composition invariants:

- `ErrNotProvided` means no applicable credential was supplied to that authenticator. The chain may continue to every later authenticator.
- A recognized but rejected credential records an authentication failure. Later authenticators still run, so another configured protocol may recognize and accept the credential.
- If no authenticator succeeds, the chain returns its recorded failures, or `ErrNotProvided` when every authenticator abstained.

`FallbackAuthenticator` invokes its fallback only when the primary authenticator returns `ErrNotProvided`. Therefore an explicitly configured anonymous fallback applies only to requests without credentials; supplied invalid credentials produce HTTP 401.

`StaticTokenAuthenticator` compares one opaque token and returns one fixed authentication value. It stores only a SHA-256 digest, copies mutable authentication fields, and compares digests in constant time. Deployment-specific subjects, groups, and authorization policy remain with the caller.

AuthenticationReview and AuthorizationReview are common transport contracts,
not IAM domain models. Their clients accept an optional transport wrapper so a
resource server can compose its OAuth Client Credentials identity around the
endpoint-specific TLS/proxy transport. AuthenticationReview audiences follow
TokenReview semantics: only token credentials may request them, the server
validates them through the authentication context, and the response reports
the validated intersection. A requested audience with no returned match is an
authentication failure.

## Audit seam

`AuditSink` is the audit delivery seam. `FanoutAuditSink` invokes every sink in
parallel for each immutable event and aggregates errors only after all
destinations have been attempted; it is fan-out rather than an ordered decision
chain. Services own which sink adapters are available in their configuration.
When delivery is asynchronous, each destination is wrapped in its own
`CachedAuditSink` before fan-out so backpressure and failure remain isolated per
destination.

`AuthenticationInfo.Clone` is an ownership operation for implementations that retain authentication across requests. Static-token and authentication-cache implementations use it before returning retained values so request-local mutation cannot alter future authentication results. Ordinary request propagation, context storage, audit, and decoded webhook or header values do not clone.

## OAuth resource server seam

An OAuth resource server accepts access tokens, not OIDC ID tokens. The JWT access-token adapter follows the RFC 9068 profile and keeps its protocol claims separate from `AuthenticationInfo`. A client-credentials token maps its verified `sub` to the subject. A delegated token maps the top-level `sub` to the subject and the outermost RFC 8693 `act` claim to the current actor. The OAuth `client_id` remains protocol information and is not used to classify a subject or infer an actor.

`Access` is non-nil only for an OAuth access token, including a token with no scopes. Audience validation happens during token verification. Scopes are authorization rules enforced by the resource server. `OAuth2ScopeAuthorizer` handles access-token requests and returns NoOpinion for other authentication modes, allowing callers to use it as one complete policy in an authorization chain. It authorizes when any granted scope matches the request attributes. The matcher parses `<action>:<resource>`, matches arbitrary actions exactly, then allows an action matcher to interpret aggregate actions such as `read` and `write`. A separate resource matcher compares the granted resource with the request's logical target. The default resource matcher uses only the final `Attributes.Resources` entry, so a parent resource does not implicitly authorize a nested target. Domains with aliases or virtual resource groups compose the standard matcher with their own resource matcher instead of generating a required scope from the request. The audience identifies the Resource Server, so the scope does not repeat a service prefix. A malformed scope or an operation without a matching target denies the access-token request. A missing matching scope returns a challenged denial that the authorization filter renders as HTTP 403 with the RFC 6750 `insufficient_scope` challenge. The access-token adapter translates a provider's invalid-token error into a challenged authentication error that the authentication filter renders as HTTP 401 with `invalid_token`.

## Authorization seam

`Authorizer` consumes the complete `AuthenticationInfo` and request `Attributes`. This preserves subject, actor, groups, audiences, and scopes through the authorization decision. Business authorizers use stable subject IDs; policies that care about delegation inspect the current actor explicitly.

`Attributes.Service` identifies the Resource Server whose operation is being
authorized and audited. `ServiceAttributesExtractor` decorates an existing
extractor at server assembly time so route parsing remains generic while each
service supplies its own stable name. Cross-service policy must not infer the
service from URL shape or leave it implicit.

`Decision` expresses only Allow, Deny, or NoOpinion. The accompanying reason is human-readable and never controls the HTTP response. An Authorizer that intentionally requires a specific denial response returns a structured `errors.Status`; the authorization filter preserves it, while ordinary evaluation errors become Forbidden. This allows resource-hiding policies to return NotFound without a magic reason string.

Authorization reasons and explicit `errors.Status` messages are public response data. Any other authorization error is diagnostic: the filter logs its details from the request context and returns only a generic Forbidden status. This avoids exposing policy, storage, or provider details without introducing another error wrapper.

`AuthorizerChain` composes alternative complete policies: Allow and Deny are final, while NoOpinion continues. A scope policy may participate when it completely decides access-token requests and later policies handle other request kinds. When a deployment requires scope and a local policy for the same request, it must provide an explicit all-of Authorizer instead of relying on the first-decisive chain. `NewAuthorizationFilter` enforces one already-composed Authorizer and does not choose policy-combining semantics.

`AuthorizationFilter` adapts the domain `Authorizer` seam to `RequestAuthorizer`, then applies the shared HTTP decision flow as a `Filter`. Its `RequestAuthorizer` implementation is intentional so domain and transport authorizers use identical prior-decision and response semantics.

## Trace seam

Domain filters do not depend on OpenTelemetry or mutate spans. `trace.go` owns HTTP tracing and optional request enrichment. `NewEndUserTraceFilter` consumes the authentication context after `NewAuthenticationFilter`; `NewAuthorizationTraceFilter` consumes request attributes after `NewAttributeExtractionFilter`. Both are explicit composition choices because end-user identifiers and authorization resource names may be sensitive or high-cardinality. Default route tracing records the route template but not dynamic path-variable values.

## Trusted propagation

Authentication webhooks and trusted request-header propagation carry the complete `AuthenticationInfo` structure. Request headers use one multi-header representation based on the Kubernetes authenticating-proxy convention. `X-Remote-User` contains the optional `Subject.Name`, `X-Remote-Uid` contains `Subject.ID`, and `X-Remote-Group` is repeated. Fields outside that convention use its `X-Remote-Extra-*` extension namespace instead of defining unrelated `X-Remote-*` headers.

`X-Remote-Extra-Actor` contains the Actor's stable ID and marks Actor presence; `X-Remote-Extra-Actor-Name` contains its optional username or principal name. `X-Remote-Extra-Audience` and `X-Remote-Extra-Scopes` are repeated fields. `X-Remote-Extra-Access: oauth2` marks a non-nil OAuth access constraint, including one with empty audiences and scopes. Header absence otherwise represents the corresponding zero or nil field. The codec maps fields without deciding whether their contents are valid authentication data. The headers are assertions, not credentials; inbound use requires a `RequestHeaderTrustVerifier`, and the trusted proxy must remove every configured client-supplied authentication header before writing its own values.

`CIDRRequestHeaderTrustVerifier` authorizes the direct connection peer by exact IP or CIDR. `TLSRequestHeaderTrustVerifier` requires a client certificate chain already verified by the HTTP server and optionally restricts its leaf Common Name. An allowlist entry of `*` permits every peer IP or every verified client certificate name respectively; it never bypasses the TLS chain requirement. These verifiers establish assertion-source trust only and do not validate decoded authentication fields.

## List pagination seam

`meta.ListOptions` and `meta.Page` are the shared list transport contract owned
by [meta](../../meta/DESIGN.md#分页请求契约).
The contract is flat and has no mode field. A positive `limit` selects
continuation pagination, where `continue` is an optional opaque token for later
batches. Otherwise a positive `size` selects page pagination and `page` values
below one normalize to one. When neither is positive, the owning service uses
its unpaginated behavior. Fields outside the selected behavior are ignored.
`GetListOptions` accepts caller-owned page, continuation, and sort options. Each
`meta.Default*Option` only writes its owned fields. `DefaultPage` and
`DefaultContinuation` observe the other behavior's non-empty intent fields so
neither default can replace an explicit request. `GetListOptions`
parses query values first and then applies options in declaration order,
allowing Default options to fill zero values. Responses omit fields
belonging to the other pagination behavior. A continuation response retains
`limit`; an omitted or empty `continue` means traversal is complete.
Collection `resourceVersion` is separate from item versions and is omitted when
the backend cannot express it as the shared integer contract.

Request-facing and list-options execution helpers select continuation when
`Limit>0`, otherwise page pagination when `Size` is positive, otherwise an
unpaginated result. Fields outside the selected behavior are ignored. Page
values below one normalize to one. In-memory continuation
applies filtering and sorting first, then uses the stable, unique, non-empty
value returned by the independent `getID` projection as the opaque cursor for
the last item in the batch. The name projection remains responsible only for
search and ordering. Object helpers use the `GetUID()` value exposed by Store
or Kubernetes objects as this identity. A
missing cursor returns ResourceExpired. This helper traverses the current
materialized list and does not provide a cross-request snapshot guarantee.

`PageFromPreparedList` is the page-only execution seam for callers that have
already completed filtering and sorting. It applies the same page and size
normalization without repeating collection preparation.
