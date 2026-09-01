# Role-based authorization

`rbac` provides store-backed roles, subject-role bindings, and an
`authz.Authorizer` implementation.

Role bindings use the globally unique `authn.Subject.ID` as their stable key.
The `user` path and response field carry that Subject ID; they do not carry a
username. `SubjectGetter` is optional display-data enrichment and resolves the
stored ID-only `authn.SubjectReference` without changing the binding key.

`Role.Authorities` contains canonical `authz.Permission` values. Each
permission selects one Service (or `*`), actions, and colon-separated resource
patterns. Scoped roles prepend their concrete scope path before matching the
operation input. An empty or different Service does not match.

Construct an `RBACAuthorizer` with the same scoped store used by the role APIs,
then compose it as an `authz.Authorizer` and install it in the normal REST
authorization filter.
