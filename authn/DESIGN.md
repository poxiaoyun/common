# Authentication model design

## Ownership

`authn` owns the canonical `Subject`, successful `Authentication` result, and
token-result metadata shared across protocols. Authorization, ownership, audit,
and protocol adapters consume these values instead of defining parallel
identity models.

`Authentication` identifies the effective Subject and an optional authenticated
Actor acting on its behalf. Its optional `Token` contains the audiences
validated for an access token and the scopes carried by it. A nil Token means
authentication did not use an access token; a non-nil empty Token retains the
authentication-method fact. Raw credentials are never retained.

`Authentication` owns its canonical JSON shape. Subject is explicitly inline,
Actor uses `actor`, and Token uses `token`; omitting Token and encoding an empty
Token object remain distinct protocol states.

Actor records verified delegation or impersonation provenance and does not
replace Subject as the effective identity. Only an authenticator may establish
Actor, using verified authentication material. Unverified request values,
credential shape, and client identifiers do not establish an Actor.

HTTP request extraction, ordered request-authenticator composition, challenges,
cookies, headers, webhooks, and filters remain in `rest/api`. Those adapters use
the canonical Authentication directly. Login, account, session, and MFA
workflows belong to applications rather than this shared identity model.

Each protocol module owns its credential inputs, verification interface,
composition, and failure semantics. HTTP Basic, access-key/secret-key pairs,
signed requests, and SSH public-key authentication therefore remain distinct
protocol seams. After successful verification, their adapters produce the
canonical `Authentication` result; raw passwords and secret keys are never
retained in it.

## Subject identity

`Subject.ID` is the complete stable identity and is globally unique across all
Subject Types. `SubjectReference` therefore contains only ID. Type is the
identity-owner-established immutable subject classification, not an OpenFGA,
SpiceDB, Cedar, or other PDP entity type. A decision-point adapter owns any
mapping from the canonical Type to its backend schema.

The zero Type is the canonical classification used by existing authentication
providers. A non-empty Type is accepted only when an identity owner establishes
it. `user` and `anonymous` are predefined common kinds; the type remains open
for other identity-owner-defined kinds. Type accompanies
an authenticated Subject as a policy fact, while authorization bindings,
ownership references, and audit correlation retain the ID-only
`SubjectReference`.

Subject Name may be resolved only within its Type and may change. Neither Name
nor `(Type, Name)` replaces Subject ID in a durable reference.

`SubjectReference` owns its canonical JSON representation (`{"id":"..."}`)
and is reused directly at protocol seams. An adapter must not introduce a
second reference type whose only behavior is copying the same ID.
Callers construct a reference explicitly as `SubjectReference{ID: subject.ID}`;
copying the ID establishes no additional invariant and therefore has no
constructor or conversion method.

Subject classification is established by authentication. Credential adapters
do not infer it from token format, OAuth grant type, client ID, username, or the
presence of an Actor.

The zero Subject Type remains valid by contract.
