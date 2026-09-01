# Authorization decisions

`authz` defines the shared authorization interfaces and values used by API
gates, resource services, and policy decision points. It does not assign
resource visibility or business querying to the decision point. It also
defines the shared versioned Policy AST used to express conditions over trusted
facts and relationships.

## Capabilities

The package separates three levels of authorization:

- `Authorizer` decides a logical operation and may return NoOpinion so another
  authorizer can decide;
- `Checker` and optional `BatchChecker` return final decisions for concrete
  resource snapshots;
- `ResourceConstraintPlanner` produces a complete candidate-resource
  constraint that a resource domain combines with its business query.

Each interface has a fixed layer contract:

| Interface | Resource input | Result role |
| --- | --- | --- |
| `Authorizer` | API target only: Service, Action, and Resource Type or raw Path. A route-derived Resource ID is optional; callers do not load Resource Properties. | Composable operation gate; may return NoOpinion. Allow permits entering the operation, not accessing a loaded resource. |
| `Checker` | One concrete Resource with non-empty Type and ID. Scope identifies its concrete ancestors; Properties is an optional allowlist of loaded policy facts. | Final decision before a protected read or effect; returns only Allow or Deny. |
| `ResourceConstraintPlanner` | One resource collection with non-empty Type, empty ID, and the containing Scope. | Complete authorization constraint applied before business sorting, pagination, and count. |

Sharing `Authentication`, `Operation`, and `EvaluationResult` does not merge
these capabilities. Their required resource completeness, permitted decisions,
and effect on the protected operation remain different contracts.

The package intentionally does not define resource ID or action search.
Resource domains own business List semantics and their finite action catalogs;
a policy decision point does not automatically own either one.

## Decision semantics

`Authorizer` returns an `EvaluationResult` whose Decision is `DecisionAllow`,
`DecisionDeny`, or `DecisionNoOpinion`. A chain continues only for NoOpinion
and denies when no authorizer decides.

`Checker` returns a final decision for one complete resource check:

- `DecisionAllow` means the protected operation may proceed;
- `DecisionDeny` means the check completed and the operation is not
  authorized;
- a non-nil error means no reliable decision was produced, so the protected
  operation must not proceed.

Unknown `Decision` values fail closed. Callers authorize only when the value is
exactly `DecisionAllow`; they must not treat every value other than Deny as
allowed.

## Scope and policy values

`ResourceReference` identifies one concrete resource by Type and ID. `Scope`
is the ordered root-to-resource sequence of those references; an empty Scope
is the authorization instance root.

`ResourceReference` and `Scope` are also their canonical JSON wire values.
They encode as `{"type":"...","id":"..."}` and
`[{"type":"...","id":"..."}]` respectively; an empty sequence is root. Protocol
adapters use these values directly rather than defining mirror structs.

`Operation`, `Resource`, and `EvaluationResult` likewise own their lower-camel
JSON fields. A raw-path Operation retains an explicit empty `resource` object;
optional path, context, resource facts, reason, and snapshot fields are omitted
when empty.

`Permission` is the shared structured permission value:

```go
permission := authz.Permission{
    Service:   "moha",
    Actions:   []string{"get", "list"},
    Resources: []string{"moha.repository"},
}
```

It is also its canonical JSON wire value. `MatchPermission` applies its Service,
Action, and colon-separated resource wildcard semantics to an `Operation`.
The value itself is not an installed Policy or an authorization
decision.

A logical API gate uses the complete authentication result without depending
on a transport request:

```go
operation := authz.Operation{
    Service: "moha",
    Action:  "get",
    Resource: authz.Resource{
        Type: "moha.repository",
    },
}
result, err := authorizer.Authorize(ctx, authentication, operation)
if err != nil || result.Decision != authz.DecisionAllow {
    // Fail closed and apply the transport's public error policy.
}
// Allow opens only the API operation gate. After loading a repository, the
// resource owner still calls Checker with its concrete ID and optional facts.
```

`Policy` is the versioned root of a recursive expression. `Any`, `All`, `Not`,
the comparison builders, and `Related` construct expression nodes.
Service-owned properties always include their service identifier; `Related` always
uses the evaluated Subject and receives a typed resource-reference value as its
object. `Policy.Validate` verifies the closed expression shape and statically
known operand types. A decision-point's policy-management module resolves
service-owned property types against its attribute catalog before installing
the Policy. Check callers never supply or select a Policy.

## Resource-domain adapter

Resource domains keep typed facts at their own interface and map them into this
generic check representation. Resource types, IDs, and actions are ordinary
strings:

```go
import (
    "xiaoshiai.cn/common/authn"
    "xiaoshiai.cn/common/authz"
)

moha := "moha"
authentication := authn.Authentication{
    Subject: authn.Subject{Type: subjectType, ID: subjectID},
}

operation := authz.Operation{
    Service: moha,
    Action:  "read",
    Resource: authz.Resource{
        Type:       "moha.repository",
        ID:         repositoryID,
        Scope: authz.Scope{{Type: "iam.organization", ID: organization}},
        Properties: authz.Properties{
            "visibility":   visibility,
            "organization": authz.ResourceReference{
                Type: "iam.organization",
                ID:   organization,
            },
        },
    },
}
result, err := checker.Check(ctx, authentication, operation)
if err != nil || result.Decision != authz.DecisionAllow {
    // Fail closed and apply the resource API's public error policy.
}
```

Do not pass complete persistence objects as properties. The resource adapter
owns an explicit allowlist of policy facts.

The installed authorization policy may use `ResourceProperty` to identify a
typed property by its owning authorization service and name. `Related` asks
whether the evaluated Subject has the named relationship to the
ResourceReference produced by its object value. Callers supply only trusted
facts; they cannot select or replace the installed Policy.

## Authorization consistency

`EvaluationResult.Snapshot` identifies the authorization state observed by the
decision point. The string value is provider-owned and opaque. Callers may
retain it and pass it back with `WithAtLeast`; they must not parse it, order it,
construct it, or assume which underlying data revisions it contains. Omitting
`WithAtLeast` accepts the provider's normal consistency behavior. When
supplied, the provider must check against that authorization state or a newer
one, or return an error.

For example, after an authorization change, its authoritative writer may expose
a snapshot. Passing it to a causally dependent check prevents that check from
observing authorization state older than the change:

```go
result, err := checker.Check(ctx, authentication, operation, authz.WithAtLeast(changedAt))
```

`AtLeast` is a freshness lower bound, not an exact-version request. The result
may therefore report a newer snapshot. The returned `Snapshot` identifies the
authorization state actually observed by that operation and can be used as a
freshness bound for a causally dependent operation. A batch check uses one
reported snapshot for the complete result; a resource constraint plan reports
the snapshot at which its complete constraint was produced.

The access snapshot identifies only the authorization state used to evaluate the
resource facts. Resource storage versioning remains owned by the resource
domain and is not part of the authorization input.

An access snapshot is not a credential, an authorization lease, or proof that
access remains allowed. A later protected operation still evaluates the current
resource and authorization state. The consistency loop becomes useful when the
authoritative authorization mutation interface exposes snapshots that callers
can pass to `WithAtLeast`.

## List integration

The resource domain owns an authorized query that combines authorization with
search, sort, pagination, count semantics, and response projection. Its
implementation normally performs one of:

1. authorization-aware predicate or JOIN in the data query;
2. provider-specific reverse lookup of a bounded authorized reference set;
3. candidate cursor scan followed by `BatchChecker` and page refill;
4. a materialized authorization-aware access index.

Reverse lookup is an implementation choice, not a generic List interface: an
unbounded authorization ID stream cannot provide correct business sorting,
pagination, or an economical exact count by itself. Every later Get, download,
pull, or mutation is checked again against the current resource and
authorization state.

When `ResourceConstraintPlanner` is available, the resource domain requests a
constraint without passing its business filter or pagination:

```go
operation := authz.Operation{
    Service: "moha",
    Action:  "read",
    Resource: authz.Resource{
        Type:  "moha.repository",
        Scope: authz.Scope{{Type: "iam.organization", ID: organization}},
    },
    Context: requestFacts,
}
plan, err := planner.PlanResourceConstraint(ctx, authentication, operation)
if err != nil {
    // Fail closed or select another complete authorized-query implementation.
}

authorizationFilter, err := translateConstraint(plan.Constraint)
if err != nil {
    // Fail closed; unsupported constraint nodes must never be ignored.
}
filter := and(businessFilter, authorizationFilter)
page, err := repository.List(ctx, filter, sort, pageRequest)
```

The resource domain maps constraint scope, resource path, property names, and
relationships to its storage model. The translation must preserve the complete
boolean expression and value types, and the resulting filter must be applied
before sorting, pagination, and Count. `ConstraintNone` and `ConstraintAll`
are constants; `ConstraintAnd`, `ConstraintOr`, and `ConstraintNot` compose
nested constraints; `ConstraintWithin`, `ConstraintPathMatches`,
`ConstraintProperties`, and `ConstraintRelated` form leaves.

For example, a planner can express public resources plus internal resources in
organizations related to the Subject as:

```go
authz.ResourceConstraint{
    Operator: authz.ConstraintOr,
    Constraints: []authz.ResourceConstraint{
        {
            Operator: authz.ConstraintProperties,
            Properties: selector.Requirement{
                Operator: selector.Equals,
                Key:      "visibility",
                Values:   []any{"public"},
            },
        },
        {
            Operator: authz.ConstraintAnd,
            Constraints: []authz.ResourceConstraint{
                {
                    Operator: authz.ConstraintProperties,
                    Properties: selector.Requirement{
                        Operator: selector.Equals,
                        Key:      "visibility",
                        Values:   []any{"internal"},
                    },
                },
                {
                    Operator: authz.ConstraintRelated,
                    Related: authz.ResourceRelationshipConstraint{
                        Relationship: authz.RelationshipReference{
                            Service: "iam",
                            Name:    "organization.member",
                        },
                        ObjectProperty: authz.PolicyAttributeReference{
                            Service:   "moha",
                            Namespace: authz.PolicyAttributeResource,
                            Name:      "organization",
                        },
                    },
                },
            },
        },
    },
}
```

Planner wire adapters call `ResourceConstraint.Validate` after mapping an
external representation. A successful `PlanResourceConstraint` result is
already valid, so business-query translators do not repeat structural
validation. A translator that cannot implement a valid node exactly returns an
error; it never drops the node or replaces it with `ConstraintAll`.

Constraint planning is optional. If the planner cannot faithfully express an
access policy, it returns an error rather than an incomplete constraint. A
resource domain may then use a different complete implementation such as
candidate cursor scanning with `BatchChecker` or an authorization-aware
index; it must not retry the business query without an authorization condition.
