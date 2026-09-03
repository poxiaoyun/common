# oidc

`oidc` 提供基于 Discovery 的 OAuth 2.0/OpenID Connect Client、令牌校验以及 RFC 7591/7592 Client Registration。

Client Registration 只提供 `RegisterClient`、`GetClientRegistration`、`UpdateClientRegistration` 和 `DeleteClientRegistration` 等协议原语。调用方自行决定是否以及如何持久化 registration，不包含运行期注册管理器。

固定 Client Credentials 通过 `ClientOptions.Authentication` 配置。一个 `Client` 共享 issuer、Client Authentication 和 Provider Discovery；调用方通过 `NewClientCredentialsTokenSource` 为每组 resource 和 scopes 创建独立 token source。`NewClientCredentialsRoundTripper` 使用绑定好的 source 为下游请求获取并缓存短期 Access Token，并实现 `httpclient.RequestAuthenticator`，因此同一身份可用于普通 HTTP 请求和 WebSocket 握手。HTTP transport 会克隆请求并覆盖出站 `Authorization`，调用方或入站请求的凭据不会被透传给目标 Resource Server。

Access Token validation 默认要求 token 或 introspection response 的 issuer
与 Discovery issuer 一致。`AccessTokenValidation.SkipIssuerCheck` 允许使用同一
Authorization Server 的可信 JWKS 和 audience 验证由不同访问域名签发的 Access
Token；它不跳过 Discovery issuer、签名、audience、token type 或有效期校验，也不影响
ID Token 校验。只有在该 Provider key set 对所有可接受 issuer 都是权威信任根时才应启用。
