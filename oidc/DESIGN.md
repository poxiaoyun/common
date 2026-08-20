# OIDC client design

Client Registration 模块只拥有 RFC 7591/7592 的请求、响应和协议约束，不拥有服务 Client 的部署、持久化、分布式协调或 Secret 轮转策略。

RFC 7592 更新由调用方从最近一次服务端响应的完整 metadata 开始。读取或更新端点返回的新 Client Secret 和 Registration Access Token 立即替换旧值；响应省略凭据时保留当前有效值。

Client Credentials 是独立能力。一个 `Client` 对应一组 issuer 和 Client Authentication，并共享 Provider Discovery 与 metadata 刷新。调用方为每组 resource 和 scopes 创建一个 `ClientCredentialsTokenSource`；每个 source 独立缓存该目标取得的 Access Token。同一个 `Client` 因此可以服务多个 Resource Server，而不会在目标之间复用 token。

Resource Server 与 scopes 在 source 创建时绑定，不由每个出站请求动态选择。Client Credentials transport 是出站请求身份的权威 owner；它在克隆请求后设置自己的 Authorization header，不能把调用方或入站请求携带的身份继续转发给目标 Resource Server。
