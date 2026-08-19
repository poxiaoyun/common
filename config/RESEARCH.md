# Configuration center API research

调研日期：2026-08-19。本文只引用产品官方文档、官方 API 参考或官方源码；先记录外部系统的事实，再给出本项目的设计推论。不同产品的 `namespace` 并不是同一个概念，不能直接按名称类比。

## 结论摘要

- 配置的稳定身份应由 `namespace + name` 组成，而且 namespace 应作为每次调用的参数。Consul HTTP API、Nacos Client OpenAPI 和 Kubernetes API 都在请求中携带 namespace；即使 Nacos Java SDK 把 namespace 绑定在 client 上，官方也明确要求跨 namespace 创建多个 client，这是一项 SDK 限制而不是领域不变量。
- 配置中心通常保存原始 bytes/string 和内容类型，不理解业务对象。对象序列化适合作为本项目 `DynamicConfig` 的调用便利层，持久化和 HTTP wire 仍应保留 JSON 原文及版本元数据。
- Not Found 没有行业统一语义：etcd 的单 key Range 表现为空结果，Consul、Nacos HTTP API 和 AWS 资源 API 使用 Not Found。若本项目选择“不报错”，仍必须用 `nil *Configuration` 区分“配置不存在”、合法 JSON `null` 和 Watch 的“没有新版本”。
- CAS/版本是可靠写入和断线续传的基础。PATCH 不是基础 KV 产品的共同能力；它应由配置服务在服务端原子执行，并与版本前置条件组合，不能退化成客户端无条件 read-modify-write。
- Watch 应区分对象版本和流 checkpoint，并明确初始快照边界。Kubernetes 的 initial events + Bookmark 是最完整的公开范式；etcd 的 revision/resume 和 Consul 的 blocking index 也都说明 checkpoint 不是业务对象本身。
- “公开读取”是授权策略，不是 namespace 名称的含义。当前产品普遍通过 ACL/IAM/RBAC 对 namespace、key prefix 或具体资源授权。开放策略必须位于配置读取之外的独立权威数据中，不能存成需要先通过同一权限检查才能读取的配置。

## etcd

etcd 是扁平的 bytes key space，没有独立 namespace 资源；范围和前缀由 `key` / `range_end` 表达。每个 value 是 bytes，KeyValue 同时携带 `create_revision`、`mod_revision` 和每个 key 的 `version`。集群级 revision 每次修改递增，形成全局逻辑时钟和 MVCC 历史。[etcd v3 API design](https://etcd.io/docs/v3.6/learning/api/)

单 key 读取也是 Range，结果通过 `kvs` 和 `count` 表达，因此“没有匹配项”天然是空结果而不是业务对象。事务是原子 If/Then/Else，可比较 key 是否存在以及其 version、mod revision 或 value，适合构造 create-only、CAS 和防止丢失更新。[etcd transaction API](https://etcd.io/docs/v3.6/learning/api/#transaction)

Watch 可以监听一个 key 或范围，从指定 `start_revision` 恢复；一个双向流可复用多个 watch。事件包含 PUT/DELETE，progress response 和 response header revision 可用作恢复进度，历史已压缩时 watch 会被取消并返回 `compact_revision`。[etcd watch API](https://etcd.io/docs/v3.6/learning/api/#watch-api)

etcd 的 RBAC 对 key 或 key range 授予 read/write 权限，并不提供“公开 namespace”语义。[etcd authentication and RBAC](https://etcd.io/docs/v3.7/op-guide/authentication/)

## Consul KV

Consul KV 的基础身份是路径 key；Enterprise namespace 通过每个请求的 `ns` query、`X-Consul-Namespace` header、ACL token namespace 或默认 namespace 选择。递归查询还可使用 `ns=*`。这直接证明 transport client 不必在构造时永久绑定单一 namespace。[Consul KV API](https://developer.hashicorp.com/consul/api-docs/kv#methods-to-specify-namespace)

KV value 是不透明数据：普通响应返回 Base64 value，`raw=true` 返回原值。单 key 不存在返回 HTTP 404。PUT 是 create/update；`cas=0` 表示仅当不存在时创建，非零值必须匹配该 key 的 `ModifyIndex`。[Consul KV read and CAS](https://developer.hashicorp.com/consul/api-docs/kv)

Consul 没有事件对象流，而是 blocking query：客户端把上次 `X-Consul-Index` 作为下一次 `index`，请求在状态可能变化或超时后返回。返回并不保证内容变化，key 不存在时官方建议至少使用 index 1 防止忙轮询。因此“轮询响应为空”和“配置不存在”必须分开建模。[Consul blocking queries](https://developer.hashicorp.com/consul/api-docs/features/blocking)

权限由 ACL token 和 `key` / `key_prefix`、Enterprise `namespace` / `namespace_prefix` 规则控制；namespace 本身不是匿名公开开关。[Consul ACL rule reference](https://developer.hashicorp.com/consul/docs/reference/acl/rule)

## Nacos

Nacos 配置的完整身份是 `namespaceId + groupName + dataId`。3.x Client OpenAPI 让调用方在每次 GET 中传这三个值，并明确面向“已经知道身份”的运行时调用方，不提供全量扫描；响应内容包含原始 `content`、`contentType`、`md5` 和 `lastModified`。[Nacos Client API](https://nacos.io/en/docs/latest/manual/user/open-api/)

Nacos Java SDK 是一个反例：namespace 是初始化属性，一个实例只能访问一个 namespace，跨 namespace 要创建不同实例。[Nacos Java SDK usage](https://nacos.io/en/docs/latest/manual/user/java-sdk/usage/#2-initialize-the-sdk) 这不妨碍本项目在更高层接口中按调用传 namespace；相反，它说明把 namespace 绑定在公共 seam 上会把特定 adapter 的限制泄漏给所有调用方。

Nacos 3.x 的推荐运行时 API 只消费已知配置；发布/删除属于 Admin API。监听面向一个已知的 `dataId + groupName + namespaceId`，长连接只通知发生变化的身份，随后仍走正常 query 获取内容。3.x HTTP Client OpenAPI 不再提供长轮询监听。[Nacos publish, query and listen](https://nacos.io/en/docs/latest/manual/user/config/publish-query-listen/)

旧版官方 OpenAPI 将读取不存在配置列为 404；配置内容是 String。公开的 publish API 是完整 content 替换，未定义 JSON PATCH 或版本 CAS。[Nacos v1 OpenAPI](https://nacos.io/en/docs/v1/open-api/)

`public` 是默认 namespace ID，不等于匿名公开。当前 Admin Config API 明确要求 namespace read/write 权限；Nacos 也警告它应部署在可信内网，不应暴露到公共互联网。[Nacos Admin API](https://nacos.io/en/docs/latest/manual/admin/admin-api/#3-nacos-config-admin-apis)、[Nacos authorization](https://nacos.io/en/docs/latest/manual/admin/auth/)

## AWS AppConfig

AWS AppConfig 把一份运行时配置定位为 application、environment、configuration profile 三元组。客户端先为这一组三元组建立 session，再反复用一次性 token 调用 `GetLatestConfiguration`；响应给出下一 token 和下一轮询间隔。已经是最新版本时，configuration body 可以为空，这表示“没有更新”而不是配置值为空或资源不存在。[AWS AppConfig data plane](https://docs.aws.amazon.com/appconfig/latest/userguide/about-data-plane.html)、[GetLatestConfiguration API](https://docs.aws.amazon.com/appconfig/2019-10-09/APIReference/API_appconfigdata_GetLatestConfiguration.html)

AWS AppConfig Agent 的 manifest 可以同时列出多份 `application:environment:configuration`，说明一个进程消费多个逻辑 scope/配置是正常需求。[AWS AppConfig Agent manifest](https://docs.aws.amazon.com/en_us/appconfig/latest/userguide/appconfig-agent-how-to-use-additional-features.html)

Hosted configuration content 是带 MIME `ContentType` 的 bytes，可以是 JSON、TOML、protobuf 或压缩数据。每次写入创建不可变新版本；可选 `LatestVersionNumber` 是防止并发覆盖的 locking token，冲突返回 409。该 API 接收完整 content，不提供文档内部 PATCH。[CreateHostedConfigurationVersion API](https://docs.aws.amazon.com/appconfig/2019-10-09/APIReference/API_CreateHostedConfigurationVersion.html)

读取部署配置需要 `appconfig:GetLatestConfiguration`，开始 session 也有独立 IAM action，并可按 configuration resource 和 tag 约束。它是身份授权模型，不是匿名公开模型。[AWS AppConfig service authorization](https://docs.aws.amazon.com/service-authorization/latest/reference/list_appconfig.html)

## Kubernetes 补充参照

Kubernetes 的 namespaced resource 路径正是 `/namespaces/{namespace}/{resource}/{name}`；ConfigMap API 支持 `PATCH /api/v1/namespaces/{namespace}/configmaps/{name}`。直接访问 API 时，应用也可以读取不同 namespace 的 ConfigMap，所以同一 client 并不天然绑定 namespace。[ConfigMap API](https://kubernetes.io/docs/reference/kubernetes-api/core/config-map-v1/)、[ConfigMap consumption](https://kubernetes.io/docs/concepts/configuration/configmap/)

Kubernetes 支持 JSON Patch 和 JSON Merge Patch，并要求需要避免丢失更新的调用结合 `resourceVersion` 条件。Watch 用 `resourceVersion` 断点续传；`sendInitialEvents=true` 会先发当前对象的 synthetic ADDED，随后用 BOOKMARK 标明初始快照完成，再继续实时事件。历史版本不可用时返回 410，客户端必须重新建立快照。[Kubernetes API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/#updates-to-existing-resources)、[Kubernetes watch and streaming lists](https://kubernetes.io/docs/reference/using-api/api-concepts/#efficient-detection-of-changes)

Namespace 是对象名和授权 scope，但具体访问仍由 Role/ClusterRole 的 resource、verb 和可选 `resourceNames` 决定。这支持“scope 与访问策略分离”的设计。[Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)

## 对本项目的设计建议

### DynamicConfig seam

建议以一份无 namespace 状态的 client 支持下面的能力；不在核心接口中保留当前没有调用需求的 List 和 Delete：

```go
type DynamicConfig interface {
    Set(ctx context.Context, namespace, name string, object any, options ...WriteOption) (*Configuration, error)
    Get(ctx context.Context, namespace, name string, object any) (*Configuration, error)
    Patch(ctx context.Context, namespace, name string, patch Patch, object any, options ...WriteOption) (*Configuration, error)
    Watch(ctx context.Context, namespace, name string) (Watcher, error)
}
```

`Set` 在模块边界 `json.Marshal(object)`；Store 和 HTTP adapter 保存、发送合法 JSON 原文。`Get` 和 Patch 的非 nil结果对象由模块反序列化。Watch 保持为原始事件流，泛型 `OnChange[T]` 集中完成对象分配、解码和 callback，避免每个 adapter 重复实现类型逻辑。

Watch 建议同时接收 `name`。去掉 List 后，运行时消费者通常监听已知配置；同一 namespace 中不同 name 可能有完全不同的 schema，namespace-wide watch 无法安全地用一个 prototype 解码。如果未来确有平台端 namespace-wide 需求，应另设 raw event/dispatcher seam，而不是削弱该强类型契约。

`Configuration` 保留 name、Store metadata、原始 Value 和版本。基础 Watch 固定把底层 initial events + Bookmark 转换为一条高层 Initial：Configuration 非 nil 表示当前值，nil 表示 missing；之后发送 Change 或 Delete。流 checkpoint 不进入公共接口，连接结束后重新 Watch 会重新取得 Initial。

### Missing 与空值

按需求可把 missing 定义为 `(*Configuration)(nil), nil`，并保持传入 object 不变。这不是把所有空状态混为一谈：

- 存在且值为 JSON `null`：返回非 nil `Configuration`，正常执行反序列化。
- 不存在：返回 nil `Configuration`，不报错。
- Watch 建立时：固定产生一条 Initial，明确表达存在或 missing。
- JSON 无法反序列化到目标类型：返回解码错误，不能伪装成 missing。

### Set、Patch 与版本

Set 无 option 表示 upsert，`IfAbsent()` 表示 create-only，`IfVersion(version)` 表示 CAS update。Patch 只作用于配置 value，而不是 Configuration envelope；支持标准 JSON Merge Patch 和 JSON Patch，成功后可把完整新值解码到调用方 object。版本比较和 patch 应在服务端/Store 的同一原子操作中完成；JSON Patch 的 `test` 失败与版本冲突应保持可区分。

### Namespace 路径与开放读取

`/namespaces/{namespace}/configurations/{name}` 与 Kubernetes 一类 namespaced resource 路径一致，也为未来其他 namespaced resources 留出了自然层级。路径复用不意味着开放策略应自动覆盖未来所有资源。

不要把 blanket `Open bool` 解释为“该 namespace 下现在和未来的所有资源都匿名可读”。IAM/HTTP authorization owner 通过独立的 `PublicReadNamespaces` 部署策略只授予 `configurations/get`；默认开放 `system`。策略不存入待授权的 Configuration，因此不会形成“先读取配置才能知道是否允许读取配置”的鉴权递归。

当前只需要公开读取时，不应顺带匿名开放 Watch、List 或任何写操作。若未来确需公开 Watch，应单独授予 `watch`，并同时考虑连接数、事件枚举和资源消耗边界。
