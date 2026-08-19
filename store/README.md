# Store 调用指南

`store` 为业务模块提供统一的对象存储接口、资源定义、作用域、查询选项和并发控制语义。业务模块应依赖 `store.Store`，并由组合根选择具体存储实现。

本文档面向 Store 调用方。直接 Store 实现必须遵守的完整行为、并发、删除、查询和 Watch 契约见 [DESIGN.md](./DESIGN.md)。

## 作用域与调用位置

先定义本次操作使用的 `store.Store`，一次性应用完整 Scope，再通过该变量执行 `Get`、`List`、`Create`、`Update`、`Patch` 或 `Delete`。不要把 `Store.Scope(...)` 内联到具体存储操作中；显式变量使有效作用域可以被直接检查和复用。

对于 `Get` 和 `List`，在调用前紧邻声明接收结果，不要在声明和存储操作之间插入无关语句或空行。

```go
storage := service.Store.Scope(
  appbase.ScopeTenant(tenant),
  appbase.ScopeApplication(application),
)

list := &store.List[ApplicationVersion]{}
if err := storage.List(ctx, list, options...); err != nil {
  return nil, err
}
```

Scope 是有序层级。普通读写只作用于完整的当前 Scope；只有需要读取后代 Scope，并且具体实现声明支持该能力时，才使用 `IncludeSubScopes`。

## 更新与并发

普通的读取—修改—写入流程必须保留读取到的 `ResourceVersion`。调用 `Get` 后修改对象再执行 `Update`，或者 `CreateOrUpdate` 返回已有对象时，不要把 `ResourceVersion` 重置为零。

只有明确需要绕过正常乐观并发控制的特殊操作才能重置 `ResourceVersion`，并且必须在赋值处用注释说明原因。

删除必须作用于调用方读取的对象时，使用 `WithResourceVersion` 或
`WithUID`；条件不匹配返回 Conflict。

只修改少数字段时优先使用原子 `Patch`。只有操作具有完整替换语义，或者新值依赖刚读取的完整对象版本时才使用 `Update`；后一种情况应保留版本，使并发变化返回 Conflict，而不是覆盖其他写入。

## 查询与错误

使用 `ListOptionsFromMeta` 将公开的 `meta.ListOptions` 转成
`[]store.ListOption`，并直接追加服务端约束后传给 `Store.List`。标准
Store option 是实现 `ApplyToXxx` 的具体值；不得传入捕获闭包，也不得依赖
完整 Options 替换的特殊顺序。HTTP 列表边界在转换前使用
`meta.DefaultSize` 和 `meta.DefaultSort` 补充请求默认值。这些 option 只应用
调用方显式声明的边界策略；Store 自身不提供默认分页 option，也不选择 Page
或 Continue 模式。

跨操作共享的选项使用同一个领域入口，例如 `WithSubScopes`、
`WithResourceVersion`、`WithFieldRequirements` 和 `WithTTL`；具体 option 类型
按需要实现多个 `ApplyToXxx`。

调用方必须把 continuation token 视为 Store 生成的不透明值，并原样传回后续查询。未指定排序时不要依赖结果顺序；需要稳定顺序时显式提供 Store 支持的排序字段。

调用可选能力前检查 `Capabilities()`。基础 CRUD、精确 Scope 和正常的乐观并发语义不是可关闭的可选能力。

保留 Store 返回的领域错误语义，例如 NotFound、AlreadyExists、Conflict、Unsupported 和 ResourceExpired。调用方可以在业务接口处映射这些错误，但不应把并发冲突或不支持的操作降级为成功。
