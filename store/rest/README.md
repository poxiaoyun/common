# REST Store

`store/rest` 提供 `store.Store` 的 HTTP adapter。

- `NewServer` 和 `NewRemoteStore` 暴露并访问通用 Store HTTP 协议。
- 对象级 List、Watch、Get、Create、Patch、Update 和 Delete 函数用于已有领域路由，接受调用方创建的具体 `store.Object` 或 `store.ObjectList`。

## List 请求选项

普通 HTTP List 使用 `ListOptionsFromRequest`，参数只传请求默认值：

```go
options, err := storerest.ListOptionsFromRequest(
	r,
	meta.DefaultSize(20),
	meta.DefaultSort("creationTimestamp-"),
)
```

业务过滤、跨 scope 等 Store 条件在转换后追加：

```go
options = append(options,
	store.WithSubScopes(),
	store.WithFieldRequirements(store.RequirementEqual("published", true)),
)
```

需要检查或归一化公开请求字段时，直接展开解析和转换，不要再次读取 query 后用
Store option 覆盖：

```go
requestOptions := api.GetListOptions(r, meta.DefaultSize(20))
requestOptions.Sort = normalizeSort(requestOptions.Sort)
options, err := store.ListOptionsFromMeta(requestOptions)
```

`meta.ListOption` 只表达请求默认值，`store.ListOption` 表达 Store 查询条件。
转换产生的 selector 错误在 HTTP 边界返回 BadRequest。未指定 `page` 时保持零值，
未指定 `continue` 时保持空值；adapter 和 Store 都不替调用方选择或改写分页模式。

Update 不清除 ResourceVersion。请求对象携带非零版本时，底层 Store 按其乐观并发契约处理；只有调用方明确传入零值时才执行无条件更新。
