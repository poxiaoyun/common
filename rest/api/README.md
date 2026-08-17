# REST API

`rest/api` provides the HTTP routing, authentication, authorization, audit, and request-context interfaces shared by services using `common`.

Authenticators implement `TokenAuthenticator`, `BasicAuthenticator`, `SSHAuthenticator`, or the request-level `Authenticator`. `StaticTokenAuthenticator` maps one opaque token to a fixed `UserInfo`, exposes the configured `Digest` and `User`, and returns `ErrNotProvided` when the token does not match so another authenticator may decide.

Callers compose authenticators with the provided chains and install the result through `NewAuthenticateFilter`. `FallbackAuthenticator` adds an explicit fallback around a completed request authenticator; use `NewFallbackAuthenticator(chain, NewAnonymousAuthenticator())` when requests without credentials should receive the anonymous identity. `AnonymousAuthenticator` remains an ordinary standalone request, token, basic, and SSH authenticator. Authorization remains a separate decision made from the verified `UserInfo`.

`AuthenticatorChain` distinguishes an absent or inapplicable credential (`ErrNotProvided`) from an applicable credential that failed validation. An authenticator must not return `ErrNotProvided` after claiming a request credential. The chain keeps trying authenticators after a validation failure, allowing another configured protocol to authenticate the request. `FallbackAuthenticator` invokes its fallback only when the completed primary authenticator returns `ErrNotProvided`, so an anonymous fallback never downgrades invalid credentials or backend failures to anonymous access.

Request adapters own credential presence. Bearer, Basic, and Session adapters return `ErrNotProvided` only when their credential is absent; once present, a credential rejected by every underlying authenticator becomes an unauthorized error.
