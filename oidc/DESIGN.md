# OIDC client design

Client Registration 模块只拥有 RFC 7591/7592 的请求、响应和协议约束，不拥有服务 Client 的部署、持久化、分布式协调或 Secret 轮转策略。

RFC 7592 更新由调用方从最近一次服务端响应的完整 metadata 开始。读取或更新端点返回的新 Client Secret 和 Registration Access Token 立即替换旧值；响应省略凭据时保留当前有效值。

Client Credentials 是独立能力。一个 `Client` 对应一组 issuer、Client Authentication、resource 和 scopes，并在本地缓存该组合取得的 Access Token。Client Credentials transport 是出站请求身份的权威 owner；它在克隆请求后设置自己的 Authorization header，不能把调用方或入站请求携带的身份继续转发给目标 Resource Server。
