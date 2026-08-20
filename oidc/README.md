# oidc

`oidc` 提供基于 Discovery 的 OAuth 2.0/OpenID Connect Client、令牌校验以及 RFC 7591/7592 Client Registration。

Client Registration 只提供 `RegisterClient`、`GetClientRegistration`、`UpdateClientRegistration` 和 `DeleteClientRegistration` 等协议原语。调用方自行决定是否以及如何持久化 registration，不包含运行期注册管理器。

固定 Client Credentials 通过 `ClientOptions.Authentication` 配置。一个 `Client` 共享 issuer、Client Authentication 和 Provider Discovery；调用方通过 `NewClientCredentialsTokenSource` 为每组 resource 和 scopes 创建独立 token source。`NewClientCredentialsRoundTripper` 使用绑定好的 source 为下游请求获取并缓存短期 Access Token。该 transport 会克隆请求并覆盖出站 `Authorization`，因此调用方或入站请求的凭据不会被透传给目标 Resource Server。
