# Authorization decision design

## Ownership

`authz` owns three authorization capabilities. `Authorizer` is the composable
operation gate used before an API operation. `Checker` and `BatchChecker` make
final decisions about concrete resource snapshots. `ResourceConstraintPlanner`
produces the complete authorization constraint for a resource collection.
`authz` also owns the shared operation target, `Permission`, constraint, and
backend-neutral versioned `Policy` values used by those capabilities. It does
not own transport extraction, resource visibility, resource lifecycle,
business query semantics, concrete policy definitions, or persistent
authorization relationships.

The resource domain owns the meaning of its actions and the trusted resource
facts supplied for checking. It enforces every decision before the protected
read or effect. The decision-point implementation owns its installed policies,
selects exactly one by resource type and action, checks resource and request
facts, and resolves the authorization relationships it owns. A caller cannot
supply, select, version, or override a policy in an authorization operation.

Every exported input and result therefore uses caller-owned resource semantics,
the canonical `authn.Subject`, or provider-owned opaque values. Changing policy
changes the decision-point implementation or its separately owned policy
management seam, not the check constructed by callers.

## Layers

`Authorizer.Authorize` receives one `AuthorizeInput`: the complete
`authn.Authentication`, target Service, Action, logical resource path, and
trusted operation Context. It returns Allow, Deny, or NoOpinion. NoOpinion
means only that this authorizer is not applicable and another authorizer may
decide. An authorizer chain resolves an all-NoOpinion result to Deny, so the API
gate fails closed. Transport adapters extract protocol facts and normalize
them into this input; `AuthorizeInput` contains no HTTP request or protocol
review type.

`Checker.Check` is the final decision seam for one concrete subject, action,
and resource snapshot. `CheckInput` includes an optional typed request IP and
service-owned trusted request properties, but contains no Policy or policy
identifier. Its `Decision` has no NoOpinion state: a missing applicable grant
is Deny, while missing required data or evaluation failure is an error.

The optional consistency lower bound controls execution rather than describing
the proposition, so it is an operation option. `BatchChecker` accepts the same
`CheckInput` values and one shared consistency option, preserving both the item
model and the batch-wide authorization snapshot invariant.

The lower-layer capabilities are independent:

- `BatchChecker` checks a finite set of concrete access propositions in input
  order. Finite resource candidates and a resource domain's finite action
  catalog can both use it without making the decision point own either catalog.
- `ResourceConstraintPlanner` partially evaluates access for a subject,
  action, and resource type using the same installed policy as `Checker`,
  producing a complete `ResourceConstraint` over candidate resource identity,
  scope, properties, and relationships. It does not search the resource store
  or produce a business page.

The three interfaces are separate capabilities. A first-layer adapter may map
an operation into a concrete `CheckInput`, but a gate Allow never replaces the
resource owner's final check after loading the current resource snapshot or
its complete planned constraint before querying a collection.

## Resource facts and installed policy

Resource types are globally unambiguous names such as `apps.application` or
`moha.repository`; a bare domain noun is not sufficient. Resources carry their
owner-supplied revision and a deliberately selected property set. Subjects use
the globally unique ID and immutable Type established by `authn`; durable
references retain ID while Type remains a policy fact. `authz` does not define
a second subject representation. A decision-point adapter maps that canonical
subject to its backend principal representation. Resource and request context
may carry properties so an adapter can evaluate policy facts without importing
domain types.

`ResourceReference` is one typed concrete resource identity. `Scope` is the
authorization instance root when its path is empty, or a root-to-resource path
of those references otherwise. Authorization policy owners establish path
validity from their resource-type hierarchy before retaining these values.

`ResourceReference` and `Scope` own their canonical JSON field names and are
used directly as wire values. Protocol adapters do not define parallel
request/response shapes merely to rename Type, ID, or Path.

`Permission` owns the canonical `service`, `actions`, and `resources` JSON
shape. `MatchPermission` matches those fields against `AuthorizeInput`; resource
patterns use colon-separated wildcard syntax. It is a declarative value, not an
installed Policy or an authorization decision. A policy-management module may
validate and compile it into its own installed policy representation.

Properties are an adapter representation, not an invitation to send complete
stored objects. Each resource domain defines a typed policy-facts value and its
adapter explicitly maps only properties registered by the applicable
decision-point implementation.

An implementation installs its complete policy catalog when it is assembled,
or obtains policy state through a separate authoritative management seam. The
check interfaces never transport a Policy AST, policy document, policy
name, or policy version. This keeps every protocol entry point on the same
rules and prevents a caller from weakening authorization by choosing a
different policy.

For a resource type and action with no installed allow policy, `Check`
returns Deny and `PlanResourceConstraint` returns the zero, match-none
constraint. Missing required trusted facts, unavailable relationship data, or
failure to check an installed policy returns an error. Policy revisions are
part of the provider-owned Access Snapshot.

`Policy.Validate` establishes the closed expression shape and every type that
can be known without an installed attribute catalog. The policy-management
module for a decision-point implementation owns resolving property types,
checking remaining operator compatibility, and installing the validated Policy.
The Policy value is not part of the resource-domain check interface and callers
cannot provide one during a check.

`ResourceConstraint` is the closed, recursive result language used by
constraint planning. It represents authorization concepts that a plain
datastore selector cannot: candidate scope containment, complete resource path
patterns, resource-property requirements, and subject-to-resource
relationships. `selector.Requirement` remains the reusable two-valued language
inside a resource-property leaf.

## Authorized resource queries

Each resource domain owns an authorized query module whose interface expresses
the domain List result. When its decision point implements
`ResourceConstraintPlanner`, the planner selects the same installed policy used
for concrete resource checks. The query module translates the returned
constraint to its datastore and combines it with the business condition using
logical AND before sorting, pagination, and count. The planner receives no
business query or pagination values and therefore does not take ownership of
those semantics.

The `ResourceConstraint` operator owns exactly one field shape:

| Operator | Active field | Meaning |
| --- | --- | --- |
| `ConstraintNone`, `ConstraintAll` | none | match no or every candidate |
| `ConstraintAnd`, `ConstraintOr` | `Constraints` | recursive conjunction or disjunction |
| `ConstraintNot` | one child in `Constraints` | negate the complete child constraint |
| `ConstraintWithin` | `Scope` | the candidate full path is at or below the scope |
| `ConstraintPathMatches` | `ResourcePath` | the candidate full path matches a structured path pattern |
| `ConstraintProperties` | `Properties` | the candidate registered properties satisfy a selector requirement |
| `ConstraintRelated` | `Related` | the current subject has a named relationship to a candidate resource-reference property |

The zero `ResourceConstraint` is `ConstraintNone` and matches no resource, so
an omitted operator fails closed. An empty `ConstraintAnd` is true and an empty
`ConstraintOr` is false. Inactive fields must be zero. The embedded
`selector.Requirement` retains its own closed operators, validation, and
missing-property semantics; resource domains map each requirement key to their
registered policy-property and storage-field vocabulary.

`ResourcePathPattern` is structured rather than carrying a provider-specific
permission string. Each path element contains a complete or wildcard Type and
an exact, wildcard, or terminally omitted ID. `"*"` is the complete-token
wildcard. An omitted ID selects the collection for the terminal Type and is not
valid in an ancestor element. `Descendants` requires every supplied element to
be a complete resource-reference pattern; an empty descendant prefix matches
all resource paths below root.

`ResourceRelationshipConstraint` names the relationship and the registered
resource property whose value is a concrete `ResourceReference`. Request-only
or literal relationship targets are resolved while planning; only a target
that varies with each candidate becomes this leaf.

The operator vocabulary is closed so every query translator can handle its
complete known vocabulary and reject a newly introduced unsupported operator.
Ignoring an operator or a populated field that is invalid for that operator,
treating translation failure as `ConstraintAll`, or applying the constraint
after pagination violates the authorization guarantee. An adapter copies any
constraint slices or values it retains after the operation returns. Wire
adapters call `ResourceConstraint.Validate` after decoding their protocol;
in-process planners establish the same invariant before returning. Downstream
domain adapters rely on the planner interface contract and do not revalidate.

Constraint planning is an optional optimization capability, not a guarantee
that every installed policy can be represented by a particular datastore. A
planner returns an Unsupported error when it cannot faithfully express the
policy in the shared grammar; a translator likewise returns an error when its
datastore cannot faithfully execute a valid node. The authorized query module may instead use a datastore
relationship join, scan stable candidate pages using `BatchChecker`, use a
bounded provider-specific reverse lookup, or query a materialized access index.
These are adapter choices behind the domain seam.

Returning an authorization ID stream is useful only when that stream has a
known practical bound; it is not the generic planner result because it cannot
by itself preserve arbitrary business sorting, pagination, or exact count.

Likewise, the resource domain owns its finite action catalog. It may use
`BatchChecker` to calculate current allowed actions, but a decision point
does not discover new domain operations for the caller.

## Consistency and ownership

`AccessSnapshot` is one provider-owned opaque token identifying the complete
authorization state observed by a decision point. It does not expose separate
relationship, attribute, policy, or index revisions because those are
implementation knowledge. A non-empty `AtLeast` requests a result no older
than that state; an implementation that cannot honor it returns an error.
Resource revision remains a resource-domain fact on `Resource` and is not
folded into the authorization snapshot.

Inputs remain caller-owned for the duration of a call. Implementations copy any
maps or slices they retain after returning. Returned collections are owned by
the caller.

Reasons are diagnostic and have no machine-readable semantics. Callers do not
branch on them or expose them without an independent public-error policy.
