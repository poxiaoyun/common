# config

`config` 定义 namespace 作用域内版本化 JSON object 配置的契约。调用方通过 `DynamicConfig` 完整替换、读取、局部修改、列举 key 或 Watch 配置；Store 与 HTTP adapter 独立实现同一接口，一个客户端可以访问多个 namespace。

```go
client := configstore.New(storage)
current, err := client.Set(ctx, "iam", "server", &ServerOptions{Listen: ":8080"})

var options ServerOptions
current, err = client.Get(ctx, "iam", "server", &options)
```

远程调用方可以直接构造 HTTP adapter：

```go
client, err := confighttp.New("https://iam.example/v1", token)
```

使用轮转凭据的服务通过 `confighttp.NewWithTransport` 包装 adapter
自身的基础 transport，不需要将短期 Access Token 物化成固定配置。

Set 默认 upsert；`config.IfVersion(0)` 表示仅在配置尚未持久化时写入，正版本表示只修改调用方已读取的版本。Patch 支持 `application/merge-patch+json` 和 `application/json-patch+json`，补丁根节点始终是配置的 `value`，缺失配置以空 object 为补丁基准。Set 和 Patch 都拒绝 `null`、数组和标量结果。

Get 找不到配置时仍返回普通 `Configuration`：Name 是请求名称、Version 为 0、Value 为空 object，并将目标对象替换为零值。`ListKeys` 只返回已持久化配置的名称和版本。

Watch 按 namespace 和 name 观察当前配置快照，第一条消息始终是当前值，即使配置不存在。需要自动解码和 callback 时使用泛型帮助函数：

```go
err := config.OnChange[ServerOptions](ctx, client, "iam", "server",
    func(ctx context.Context, options ServerOptions, version int64) error {
        return apply(options)
    })
```

Watch 不暴露 initial、change 或 delete 类型。底层删除表现为 Version 0 的空 object 快照；每次 callback 都得到从零值重新解码的独立对象。

Store 调用方使用 `xiaoshiai.cn/common/config/store`，并通过该包的 `AddToSchema` 注册持久化类型；HTTP 调用方使用 `xiaoshiai.cn/common/config/http`。`config/noop` 表示未启用配置中心。启动配置中心参数由 `config/commandsource.Options{Address, Token}` 统一承载并按 Address scheme 选择 adapter；Address 默认空并选择 Noop。namespace 由每次调用显式传入，不是需要预先创建的对象。契约与 adapter 都不决定哪些 namespace 可以公开读取；公开策略由承载 HTTP 路由的应用负责。
