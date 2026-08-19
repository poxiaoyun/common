# config

`config` 定义 namespace 作用域内版本化 JSON 配置的契约。调用方通过 `DynamicConfig` 完整替换、读取、局部修改或 Watch 配置；Store 与 HTTP adapter 独立实现同一接口，一个客户端可以访问多个 namespace。

```go
client := configstore.New(storage)
current, err := client.Set(ctx, "iam", "server", &ServerOptions{Listen: ":8080"})

var options ServerOptions
current, err = client.Get(ctx, "iam", "server", &options)
```

远程调用方可以直接构造 HTTP adapter：

```go
client, err := confighttp.New(ctx, "https://iam.example/v1", token)
```

Set 默认 upsert，`config.IfAbsent()` 表示仅创建，`config.IfVersion(version)` 表示只更新该版本。Patch 支持 `application/merge-patch+json` 和 `application/json-patch+json`，补丁根节点始终是配置的 `value`。Get 找不到配置时返回 `(nil, nil)`，目标对象保持原值。

Watch 按 namespace 和 name 观察一个配置，第一条事件固定为 Initial。需要自动解码和 callback 时使用泛型帮助函数：

```go
err := config.OnChange[ServerOptions](ctx, client, "iam", "server",
    func(ctx context.Context, change config.Change[ServerOptions]) error {
        return apply(change.Object)
    })
```

initial missing 和 Delete 的 `Object` 为 nil；每次有值的事件都会分配一个新的对象。

Store 调用方使用 `xiaoshiai.cn/common/config/store`，并通过该包的 `AddToSchema` 注册持久化类型；HTTP 调用方使用 `xiaoshiai.cn/common/config/http`。`config/noop` 表示未启用配置中心。启动配置中心参数由 `config/commandsource.Options{Address, Token}` 统一承载并按 Address scheme 选择 adapter；Address 默认空并选择 Noop。namespace 由每次调用显式传入，不是需要预先创建的对象。契约与 adapter 都不决定哪些 namespace 可以公开读取；公开策略由承载 HTTP 路由的应用负责。
