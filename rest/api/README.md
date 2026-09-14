# REST API

`rest/api` provides the HTTP routing, authentication, authorization, audit, and request-context interfaces shared by services using `common`.

Path syntax is owned by [`rest/matcher`](../matcher/README.md). Use `{name}`
for a single segment, `{name...}` for multiple segments, a trailing `/` for an
anonymous subtree, and `{$}` for an exact end. `/v2/` does not match `/v2`;
register both when both addresses should be served. Routing never cleans or
redirects the request URL. `PathVars` returns captures decoded exactly once.
Group prefixes compose with route fragments while preserving the final slash
and end marker; a fragment without a leading slash remains a composite segment.
`Route.Path` retains the declared pattern after registration. OpenAPI projects
its documentation template separately.

`Route.ContentType` and `Group.ContentType` match the request Content-Type;
`Route.Accept` and `Group.Accept` match acceptable response media types.
Different media variants can share a path and method. A mismatch continues
looking for another route, including a less specific path. Selected handlers
and route filters run once; their errors never trigger fallback.

Set `Route.Priority` when overlapping paths need an explicit preference.
Higher values precede lower values among matching routes; the default is zero.
Equal priorities retain the existing path specificity, including static
subtrees before competing dynamic paths. A higher-priority route whose method
or media conditions do not match cannot block a lower-priority route.

Within the same path and priority, explicit methods precede `Any`. More specific
Content-Type conditions precede broader or omitted conditions. Response media
conditions precede an unrestricted response; variants then follow the client's
Accept quality, matching range specificity, declared response specificity, and
registration order. Accept supports lists, repeated header fields, wildcards,
parameters, and q values, including specific q=0 exclusions. An omitted Accept
accepts any response type; an empty or malformed Accept cannot satisfy a
response media condition. A missing, malformed, repeated, or wildcard
Content-Type cannot satisfy a request media condition. Route conditions support
`type/*` and `*/*`; declared parameters must match, and charset values are
case-insensitive. Other parameter values retain their case.

Media values within one declaration are alternatives. Parent and child group
conditions intersect across every level; invalid conditions or an empty
intersection fail registration. Expanded routes expose normalized effective
`ContentTypes` and `Accepts` values for documentation. Register matching conditions
before serving requests. The router retains the Route so plugins can install
its filters during construction.

When no route matches, the router returns 405 with Allow if some route matches
all conditions except method; otherwise it returns 404. Automatic OPTIONS
reports host/path methods independently of media headers, unless an explicit
OPTIONS or `Any` route matches. Media mismatch does not generate 406 or 415.
Media-sensitive responses include the corresponding Vary fields, including
fallback and error responses. Handlers still own response encoding and its
Content-Type; route declarations do not encode the response.

`ServeTLS` loads its certificate and key before listening and returns any
configuration error. It re-reads the pair for new TLS handshakes at most once
per minute; canceling the serving context shuts down the listener, with no
separate certificate-watcher lifecycle.

`ContentResponse` represents local content or a redirect. Local content with a
`Content-Range` header must already contain the selected bytes and is written as
HTTP 206; other local content is passed to `ServeContent` for conditional and
range request handling. Call `ServePartialContent` directly for already-selected
bytes. It preserves existing `Content-Range` and `Content-Length` headers and
uses the corresponding arguments only when those headers are absent. Pass zero
content length to omit the generated length header. The reader is always copied
to EOF without validating its bytes against the declared length.

Authenticators normalize HTTP credentials into the canonical
[`authn.Authentication`](../../authn/README.md), exposed here through the
`Authentication` alias. It contains the effective Subject, an optional Actor,
and optional verified access-token metadata. `HTTPAuthenticator` authenticates
a complete request through `AuthenticateHTTP(w, r)`. Credential adapters retain
operation-specific interfaces such as `AuthenticateToken` and
`AuthenticateBasic`.
`Subject.ID` is the complete globally unique stable identity. `Subject.Type` is
its immutable classification and remains available as an authorization fact;
it is not part of a durable identity reference. `Subject.Name` is the
provider-verified username or principal name, and `Subject.DisplayName` is a
non-unique human-facing label. `rest/api.Subject` is a Go alias to the
authn-owned Subject, not a separate identity model.

Callers compose authenticators and install the result through `NewAuthenticationFilter`. `FallbackAuthenticator` adds an explicit fallback around a completed request authenticator; use `NewFallbackAuthenticator(chain, NewAnonymousAuthenticator())` when requests without credentials should receive the anonymous subject. An invalid supplied credential is never downgraded to anonymous.

`AuthenticationChallengeError` carries a public response status and `WWW-Authenticate` value through authenticator and authorizer composition. Provider adapters log diagnostic errors before translating them into this shared response error. The final HTTP error writer writes the challenge only after the request is rejected. `NewBearerTokenAuthenticationFilter` returns a bare `Bearer` challenge when no more specific challenge is present. Invalid OAuth access tokens add `error="invalid_token"`, while insufficient scope returns HTTP 403 with `error="insufficient_scope"`.

`authz.Authorizer` receives the complete `Authentication` and an
`authz.Operation` as separate arguments. OAuth scopes are access-token
authorization rules.
`OAuth2ScopeAuthorizer{}` parses the default `<action>:<resource>` convention
and matches each granted scope against the operation. Arbitrary actions such as
`create` or `publish` match exactly; `read` covers `get`, `list`, and `exists`,
while `write` covers other non-empty actions. `NewOAuth2ScopeMatcher` composes a
different aggregate-action matcher or logical-resource matcher. The default resource
matcher uses only the final operation resource, so parent resources do not
authorize nested targets. Compose it with other complete alternative policies
through `authz.AuthorizerChain`; deployments that require both scopes and local
policy must provide an Authorizer with those explicit combining semantics
before installing `NewAuthorizationFilter`.

Wrap a route extractor with `ServiceAttributesExtractor("cloud", extractor)`
when authorization and audit policy must identify the target Resource Server.
The wrapper sets `Attributes.Service`; the wrapped extractor continues to own
action and resource parsing.

The default REST extractor maps `HEAD` requests to `exists` for both resources
and collections, unless the URL declares an explicit action. An exact
`exists:resource` scope permits existence checks without granting `GET` access.

`rest/api` converts the final extracted resource into `authz.Resource` and its
parents into `authz.Scope`. A route-derived final name is carried as the
optional Resource ID without loading resource facts. `authz.MatchPermission`
owns comparison with the shared `authz.Permission`.

Authorization reasons are descriptive only. To intentionally return a specific denial status, such as hiding a resource with HTTP 404, an Authorizer returns the corresponding `common/errors.Status`; untyped evaluation errors are returned as HTTP 403.

`CheckerAuthorizer` adapts the model-independent `common/authz.Checker` to
`authz.Authorizer`. Callers provide a `BuildCheckOperationFunc` because the
logical operation cannot infer a resource domain's authoritative entity ID or
policy facts.
Use this adapter for a PDP-backed operation gate; an Allow does not replace the
concrete resource check owned by a handler or domain module after it loads the
current resource snapshot.

Authentication and authorization errors are diagnostic by default: filters record their details through the logger in the request context and return generic 401 or 403 responses. Reasons and explicit `common/errors.Status` messages are considered intentionally public and must not contain secrets.

Authentication and authorization filters do not write trace data. OpenTelemetry behavior is owned by `trace.go` and composed explicitly: install `NewEndUserTraceFilter` after authentication and `NewAuthorizationTraceFilter` after request attributes only when those potentially sensitive or high-cardinality attributes are required. Route tracing records the low-cardinality `http.route` template and does not record dynamic path-variable values.

`StaticTokenAuthenticator` maps one opaque token to a fixed
`Authentication`. Request-header and webhook adapters transport the
same canonical value without provider-specific attribute maps. Trusted
request-header propagation uses one multi-header representation based on the
Kubernetes authenticating-proxy convention: `X-Remote-User`, `X-Remote-Uid`,
and repeated `X-Remote-Group` carry the standard fields. Subject and Actor Type
use `X-Remote-Extra-Subject-Type` and `X-Remote-Extra-Actor-Type`; display name,
email, Actor, Token metadata, audiences, and scopes use the remaining fixed
`X-Remote-Extra-*` fields. `X-Remote-Extra-Access: oauth2` distinguishes a
non-nil empty Token from authentication without an access token.

Proxy transports clone the outbound request and remove all configured authentication headers before either injecting fixed/context authentication or forwarding a route without authentication. This prevents client-supplied assertions from crossing either protected or public proxy routes.

`AuthenticationReview` and `AuthorizationReview` are shared wire contracts.
The webhook authenticator, authorizer, and audit sink can receive an
`httpclient.TransportWrapper`, allowing a service to apply one OAuth Client
Credentials transport to all calls to an IAM Resource Server while retaining
each endpoint's own timeout, proxy, and TLS settings. Token authentication
reviews may request audiences; the response must contain at least one validated
requested audience. Basic and SSH reviews are audience-unaware.

`AuthorizationClient{Client: client}` uses a client whose base URL is the API
root (for example `https://iam/v1`) and implements all four `authz` capabilities:

| Method | POST endpoint | Successful result |
| --- | --- | --- |
| `Authorize` | `/authorization-reviews` | Allow, Deny, or NoOpinion |
| `Check` | `/authorization-checks` | Final Allow or Deny |
| `BatchCheck` | `/authorization-batch-checks` | One final decision per input, in order |
| `PlanResourceConstraint` | `/authorization-constraint-plans` | Validated complete constraint |

Requests carry `spec.authentication` and the complete `spec.operation` (or
`spec.operations` for a batch). Check, batch, and plan requests may include
`spec.atLeast`; responses carry `status` with the canonical `authz` result.
Resource facts use the [typed authz JSON representation](../../authz/README.md#typed-json-facts).
The calling service's HTTP credential is separate from the evaluated Subject;
only trusted callers may assert authentication and authoritative resource facts.
No request accepts caller-selected Policy rules.

Structured non-success HTTP statuses remain `common/errors.Status` errors.
Missing results, malformed constraints, invalid final decisions, and incomplete
batches return errors with no usable result. `WebhookAuthorizer` remains an
operation-gate client configured with the exact review endpoint URL, not an API
root; use `AuthorizationClient` for the other capabilities.

`NewDefaultAuditOptions` collects HTTP audit events for POST, PUT, PATCH, and
DELETE. `SimpleAuditor.OnRequest` skips other methods before capturing request
or response data, including when used through `NewAuditFilter` or a custom
auditor. Excluded requests never reach a sink. Set `RecordStatusMethods` explicitly
when a service needs to audit reads or additional methods; an empty list audits
all methods. Body-method options control payload capture, not event selection.
`WhiteList` excludes matching paths before capture and delivery.

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
