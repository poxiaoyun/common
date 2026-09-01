# RBAC design

## Ownership

`rbac` owns persisted roles, Subject-to-role bindings, scope inheritance, and
the evaluation of those bindings. `authn` owns canonical Subject identity,
`authz` owns the operation input, structured Permission value, and permission
matching. `rest/api` only adapts HTTP request attributes into that input.

## Stable identity

`UserRole.ObjectMeta.Name` is the globally unique stable Subject ID. Every
mutation, lookup, and authorization query uses that value; usernames and
display data never become binding keys. `SubjectGetter` receives an ID-only
`authn.SubjectReference` solely to enrich list responses.

## Permission evaluation

`Role.Authorities` is the persisted permission collection. Each entry is an
`authz.Permission`; its Service is matched before its actions and resources.
Scoped evaluation prefixes each resource pattern with the concrete binding
scope, then delegates operation matching to `authz.MatchPermission`.
Permissions are alternatives: a non-matching entry never prevents a later
entry from allowing the request.
