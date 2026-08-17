# REST API design

## Authentication seam

Authentication converts transport credentials into `AuthenticateInfo`. At the request seam, an authenticator returns `ErrNotProvided` only when the request does not contain a credential applicable to that authenticator, allowing an authentication chain to try another adapter. Credential-level adapters may return `ErrNotProvided` so another adapter can inspect the same credential, but the request adapter that established credential presence converts final non-recognition into an authentication error. Authorization consumes the resulting `UserInfo` through a separate interface.

`AuthenticatorChain` preserves these ordered-composition invariants:

- `ErrNotProvided` means no applicable credential was supplied to that authenticator. The chain may continue to every later authenticator.
- A recognized but rejected credential records an authentication failure. Later authenticators still run, so another configured protocol such as OIDC or Webhook may recognize and accept the credential.
- If no authenticator succeeds, the chain returns its recorded failures, or `ErrNotProvided` when every authenticator abstained.

These rules distinguish absence from invalidity at the shared authentication seam: no credentials may become anonymous when configured, while supplied invalid credentials must produce HTTP 401. OAuth 2.0 Bearer failures use the RFC 6750 challenge contract: missing credentials return a Bearer challenge, invalid access tokens return `invalid_token`, and denied OAuth Client identities return `insufficient_scope`.

`FallbackAuthenticator` owns conditional fallback composition. It invokes its fallback only when the primary authenticator returns `ErrNotProvided`; primary successes and all other errors pass through unchanged. Anonymous authentication remains an ordinary authenticator and becomes conditional only when a caller explicitly installs it as the fallback around a fully composed request authenticator.

`StaticTokenAuthenticator` owns the generic mechanism for comparing one opaque token and returning one fixed user identity. Its exported `Digest` and `User` fields expose the configured authentication state. The constructor stores only a SHA-256 digest and copies mutable identity fields; authentication compares digests in constant time and returns a fresh identity copy. Deployment-specific principal names, groups, configuration, and authorization policy remain with the caller.
