# Authentication

`authn` owns the canonical authenticated identity, successful authentication
results, and token-result metadata shared by protocol adapters.

`Subject.ID` is globally unique and forms the stable identity.
`SubjectReference` therefore contains only ID. Type is the immutable
identity-owner-established classification used as an authentication and policy
fact. The predefined types are `SubjectTypeUser` and `SubjectTypeAnonymous`;
callers must not infer Type from a credential shape, username, or PDP schema.

```go
subject := authn.Subject{ID: "alice", Type: authn.SubjectTypeUser}
reference := authn.SubjectReference{ID: subject.ID}
```

Durable authorization and ownership records retain `reference`, not Type,
Name, or DisplayName. Type remains available on the authenticated Subject for
policy evaluation.

`SubjectReference` is also its canonical JSON wire value and encodes as an
object containing the lowercase `id` field. Protocol adapters reuse it directly
instead of defining request and response mirrors.

Credential inputs and verification interfaces belong to the protocol adapter.
After verification, the adapter produces the canonical result:

```go
authentication, err := tokens.AuthenticateToken(ctx, rawToken)
if err != nil {
    return err
}
subject := authentication.Subject
```

`Authentication` contains the effective Subject and an optional Actor acting on
its behalf. Actor is nil for direct authentication and must come from delegation
or impersonation information verified by the authenticator; callers must not
populate it from unverified request values or infer it from a client identifier.
For access-token authentication, `Token` is non-nil and contains the validated
audiences and token scopes. A non-nil empty `TokenInfo` still records that an
access token was used.

`Authentication` is also its canonical JSON wire value. Embedded Subject
fields are inline; `actor` and `token` are optional objects. An omitted `token`
means authentication did not use an access token, while an empty `token` object
preserves access-token authentication without audiences or scopes.

Token, username/password, access-key/secret-key, request-signature, and SSH
public-key verification remain separate protocol seams. An access key selects a
credential record and is not a Subject ID. Secret keys and passwords must not be
retained or logged. SSH adapters establish private-key possession before
producing `Authentication`; the SSH username is a protocol claim, not a Subject
ID. Request parsing, challenges, and handshake negotiation remain with their
protocol adapters.
