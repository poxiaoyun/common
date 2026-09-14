# REST API design

## Route selection

Path matching is fully described by the path pattern. A trailing slash matches
that path and its descendants; `{$}` requires the path to end there. Named
captures use `{name}` for one segment and `{name...}` for multiple segments.
Composite segments, regular-expression constraints, and middle multi-segment
captures are extensions owned by the matcher. Routing never cleans or redirects
the request URL. Group composition preserves the final pattern's delimiters.
The declared pattern remains authoritative after registration; OpenAPI derives
its path template without modifying that declaration.

A route is selected by host, path, method, request Content-Type, and acceptable
response media types. Media conditions belong to routing, not to a validation
filter. A rejected candidate has no effects: it does not consume the body,
install path variables, execute route filters, or write a response. Once a
handler is selected, its response is final and never triggers another route.
Global server filters retain their independent lifecycle around the router.

Route Priority resolves overlapping paths when callers need an explicit
preference. Higher values take precedence among fully matching routes; zero is
the default. Equal priorities retain the existing left-to-right path
specificity, including a static subtree before a competing dynamic path.
Within the same path and priority, an explicit method precedes an any-method
route. Request media specificity takes priority,
followed by a declared response condition over an unrestricted response.
Acceptable response quality, matching range specificity, and declared response
specificity resolve competing variants, followed by registration order. A media mismatch
continues selection at the same path and then at less specific paths. An
explicit host retains its own routing namespace.

Selection examines the path candidates without invoking their handlers. The
chosen route owns a copy of its captured variables, so later candidate matching
cannot overwrite them. A higher-priority candidate that fails a method or media
condition does not prevent a lower-priority match.

Content-Type describes one representation and must be present when the route
constrains that header. Accept is a list of media ranges with parameters and quality values;
its most specific applicable range determines each representation's quality,
including exclusion by q=0. An omitted Accept imposes no client preference.
Media parameters in a condition must match, while unspecified parameters do
not restrict the request. Conditions in one declaration are alternatives;
parent and child group conditions intersect. Invalid declarations and empty
intersections fail route registration. The expanded Route exposes the effective
conditions used by routing and documentation.

After all candidates fail, a request receives 405 only when a candidate matches
every condition except method; otherwise it receives 404. Allow contains the
methods of those compatible candidates. Automatic OPTIONS describes all
methods available at the host and path, independently of request media, after
explicit OPTIONS and any-method routes have had an opportunity to match.
Media selection itself never writes 406 or 415. Responses selected through
media conditions vary on the corresponding request headers so caches cannot
reuse a result across incompatible routing decisions.

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

Authentication converts HTTP credentials into the canonical
`authn.Authentication`; `rest/api.Authentication` is an alias for callers.
`authn` owns Subject, Actor provenance, Token metadata, and the complete
successful authentication result.
`rest/api` owns HTTP extraction, ordered request-authenticator composition,
propagation, challenges, filter behavior, and the explicit OAuth header marker.
`rest/api.Subject` is a Go alias for existing callers, not another model.
`Subject` is the identity the request is about.
An optional `Actor` is the current identity acting for that subject. An optional
`Token` contains audiences and scopes verified from an OAuth 2.0 access
token. Groups belong to a subject; audiences and scopes belong to the credential
used for this request. Protocol claims and provider-specific metadata do not
flow through an unstructured attribute map.

Context helpers are named for the value they carry: `WithAuthentication` pairs with `AuthenticationFromContext`, and `WithAuthorizationDecision` pairs with `AuthorizationDecisionFromContext`. Authentication operations retain the verb `Authenticate`; values and filters use the noun `Authentication`.

`Subject.ID` is the complete globally unique stable authorization, ownership,
and audit identity. Type is its immutable classification and remains available
as a policy fact; it is not part of a durable identity reference. The zero Type
is the authn-defined default classification.
`Name` is the provider-verified username or principal name within the
authentication domain, while `DisplayName` is a non-unique human-facing label.
Neither is a stable ownership key. An OIDC adapter must resolve the issuer and
`sub` pair into the authentication domain's canonical Subject, map
`preferred_username` to `Name`, and map the OIDC `name` claim to `DisplayName`.
It must not infer Subject Type from provider-specific ID prefixes, token format,
or comparisons with `client_id`.

At the request seam, an authenticator returns `ErrNotProvided` only when the request does not contain a credential applicable to that authenticator. Credential-level adapters may return `ErrNotProvided` so another adapter can inspect the same credential, but the request adapter that established credential presence converts final non-recognition into an authentication error.

An authentication or authorization adapter that requires an HTTP authentication challenge returns `AuthenticationChallengeError` with the public response status and challenge value. The adapter logs any diagnostic provider error before translating it. The challenge travels through authenticator and authorizer composition without writing response state. The final HTTP error writer is the sole owner that copies the challenge to `WWW-Authenticate` when the request is rejected; generic response handling must not depend on provider packages.

Authentication failures separate public response data from diagnostic errors. An explicit `errors.Status` or challenge is public response data. For any other error, the default authentication filter logs its details from the request context and returns a generic Unauthorized status. A custom `AuthenticationErrorHandlerFunc` owns both logging and response redaction.

`HTTPAuthenticator` uses `AuthenticateHTTP` because its seam is a complete
HTTP request. Credential-level interfaces use operation-specific names such as
`AuthenticateToken` and `AuthenticateBasic`.

`HTTPAuthenticatorChain` preserves these ordered-composition invariants:

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

Authorization transports preserve the complete canonical Authentication and
Operation, including typed resource facts and request Context. The authenticated
HTTP caller is the assertion source, not the evaluated Subject; the receiving
service owns authorizing that caller to submit these trusted assertions. Check
requests never contain an installed Policy or policy selector.

Operation review, final resource Check, ordered BatchCheck, and collection
constraint planning have separate endpoints and result contracts. Successful
responses contain a non-null result. A final check permits only Allow or Deny;
a batch has exactly one such decision per input in the original order. A plan
must decode to a structurally valid complete constraint. The transport adapter
rejects malformed results rather than allowing them to enter the domain.
Evaluation failures use structured non-success HTTP statuses; decision reasons
never select an error type. Optional AtLeast values travel without interpretation.

## Resource authorization adapter

`authz.Authorizer` is the operation gate. `rest/api` extracts HTTP
`Attributes`, maps them to `authz.Operation`, invokes the authorizer with the
canonical `authn.Authentication`, and maps its result to the HTTP response
policy. HTTP method and path remain transport facts in `Operation.Context`;
they are not identity fields on the authz-owned operation model.

Default REST action extraction maps `HEAD` on a resource or collection to
`exists`. An explicit action in the URL remains authoritative. Existence is a
distinct authorization action: the aggregate OAuth `read` scope includes
`get`, `list`, and `exists`; aggregate `write` excludes all three. An exact
`exists` scope does not grant content reads or listing.

`CheckerAuthorizer` adapts an `authz.Checker` to `authz.Authorizer`. Its caller
supplies `BuildCheckOperationFunc`, which owns enriching the logical Operation
with the resource-server-specific authoritative ID and policy facts required
by Checker.
There is no default mapping: URL resources are not globally unambiguous entity
types, and request attributes do not contain resource Visibility, ownership,
revision, or other policy facts.

The adapter accepts a final operation-gate Allow or Deny and maps unknown PDP
decisions to Deny. An adapter Allow does not establish authorization for a
resource that is loaded or mutated later by the handler. The resource domain
still evaluates the concrete current resource before every protected read or
effect. Request-filter decision reuse never replaces that domain enforcement.

## Audit seam

`AuditSink` is the audit delivery seam. `FanoutAuditSink` invokes every sink in
parallel for each immutable event and aggregates errors only after all
destinations have been attempted; it is fan-out rather than an ordered decision
chain. Services own which sink adapters are available in their configuration.
When delivery is asynchronous, each destination is wrapped in its own
`CachedAuditSink` before fan-out so backpressure and failure remain isolated per
destination.

Static-token, authentication-cache, and fixed proxy implementations privately
copy mutable authentication fields they retain. `authn.Authentication` exposes
no copying operation.

## OAuth resource server seam

An OAuth resource server accepts access tokens, not OIDC ID tokens. The JWT access-token adapter follows the RFC 9068 profile and keeps its protocol claims separate from `Authentication`. A client-credentials token maps its verified `sub` to the subject. A delegated token maps the top-level `sub` to the subject and the outermost RFC 8693 `act` claim to the current actor. The OAuth `client_id` remains protocol information and is not used to classify a subject or infer an actor.

`Token` is non-nil only for access-token authentication, including a token with
no scopes. Audience validation happens during token verification. Scopes are
authorization rules enforced by the resource server. `OAuth2ScopeAuthorizer`
handles access-token requests and returns NoOpinion for other authentication
modes, allowing callers to use it as one complete policy in an authorization
chain. It authorizes when any granted scope matches the request attributes. The
matcher parses `<action>:<resource>`, matches arbitrary actions exactly, then
allows an action matcher to interpret aggregate actions such as `read` and
`write`. A separate resource matcher compares the granted resource with the
request's logical target. The default resource matcher uses only the final
`Attributes.Resources` entry, so a parent resource does not implicitly
authorize a nested target. Domains with aliases or virtual resource groups
compose the standard matcher with their own resource matcher instead of
generating a required scope from the request. The audience identifies the
Resource Server, so the scope does not repeat a service prefix. A malformed
scope or an operation without a matching target denies the access-token
request. A missing matching scope returns a challenged denial that the
authorization filter renders as HTTP 403 with the RFC 6750
`insufficient_scope` challenge. The access-token adapter translates a
provider's invalid-token error into a challenged authentication error that the
authentication filter renders as HTTP 401 with `invalid_token`.

## Authorization seam

`authz.Authorizer` receives the complete canonical `authn.Authentication` and
`authz.Operation` as separate arguments. This preserves subject Type and ID,
actor, groups, audiences, and scopes through the authorization decision.
Business authorizers persist the globally unique Subject ID and may inspect
Type as a policy fact; policies that care about delegation inspect the current
actor explicitly.

`Attributes.Service` identifies the Resource Server whose operation is being
authorized and audited. `ServiceAttributesExtractor` decorates an existing
extractor at server assembly time so route parsing remains generic while each
service supplies its own stable name. Cross-service policy must not infer the
service from URL shape or leave it implicit.

Structured permission fields and their operation matching are owned by
`authz.Permission`. `rest/api` maps the extracted HTTP resource path to one
terminal `authz.Resource` whose `Scope` contains its parents; it does not own a
second permission matcher or define an Authority value.

`authz.Decision` expresses only Allow, Deny, or NoOpinion. The
accompanying reason is human-readable and never controls the HTTP response. An
Authorizer that intentionally requires a specific denial response returns a
structured `errors.Status`; the authorization filter preserves it, while
ordinary authorization errors become Forbidden. This allows resource-hiding
policies to return NotFound without a magic reason string.

Authorization reasons and explicit `errors.Status` messages are public response data. Any other authorization error is diagnostic: the filter logs its details from the request context and returns only a generic Forbidden status. This avoids exposing policy, storage, or provider details without introducing another error wrapper.

`authz.AuthorizerChain` composes alternative complete policies: Allow and Deny
are final, while NoOpinion continues. A scope policy may participate when it
completely decides access-token requests and later policies handle other
request kinds. When a deployment requires scope and a local policy for the same
request, it must provide an explicit all-of Authorizer instead of relying on
the first-decisive chain. `NewAuthorizationFilter` enforces one already-composed
Authorizer and does not choose policy-combining semantics.

`AuthorizationFilter` adapts `authz.Authorizer` to the HTTP-only
`RequestAuthorizer`, then applies the shared HTTP decision flow as a `Filter`.
Its `RequestAuthorizer` implementation is intentional so domain and transport
authorizers use identical prior-decision and response semantics.

## Trace seam

Domain filters do not depend on OpenTelemetry or mutate spans. `trace.go` owns HTTP tracing and optional request enrichment. `NewEndUserTraceFilter` reads the authentication context after `NewAuthenticationFilter`; `NewAuthorizationTraceFilter` reads request attributes after `NewAttributeExtractionFilter`. Both are explicit composition choices because end-user identifiers and authorization resource names may be sensitive or high-cardinality. Default route tracing records the route template but not dynamic path-variable values.

## Trusted propagation

Authentication webhooks and trusted request-header propagation carry the
complete `rest/api.Authentication` structure. Request headers use one
multi-header representation based on the Kubernetes authenticating-proxy
convention. `X-Remote-User` contains the optional `Subject.Name`,
`X-Remote-Uid` contains `Subject.ID`, and `X-Remote-Group` is repeated. Fields
outside that convention use its `X-Remote-Extra-*` extension namespace instead
of defining unrelated `X-Remote-*` headers.

`X-Remote-Extra-Subject-Type` and `X-Remote-Extra-Actor-Type` carry the canonical
Subject types. `X-Remote-Extra-Actor` contains the Actor's stable ID and marks
Actor presence; `X-Remote-Extra-Actor-Name` contains its optional username or
principal name. `X-Remote-Extra-Audience` and `X-Remote-Extra-Scopes` are
repeated fields. `X-Remote-Extra-Access: oauth2` marks a non-nil OAuth access
constraint, including one with empty audiences and scopes. Header absence
otherwise represents the corresponding zero or nil field. The codec maps
fields without deciding whether their contents are valid authentication data.
The headers are assertions, not credentials; inbound use requires a
`RequestHeaderTrustVerifier`, and the trusted proxy must remove every configured
client-supplied authentication header before writing its own values.

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
