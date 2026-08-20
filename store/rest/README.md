# REST Store

`store/rest` 提供 `store.Store` 的 HTTP adapter。

- `NewServer` 和 `NewRemoteStore` 暴露并访问通用 Store HTTP 协议。
- 对象级 List、Watch、Get、Create、Patch、Update 和 Delete 函数用于已有领域路由，接受调用方创建的具体 `store.Object` 或 `store.ObjectList`。

## List 请求选项

普通 HTTP List 使用 `ListOptionsFromRequest`，参数只传请求默认值：

```go
options, err := storerest.ListOptionsFromRequest(
	r,
	meta.DefaultPage(1, 20),
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
requestOptions, err := api.GetListOptions(r, meta.DefaultPage(1, 20))
if err != nil {
	return err
}
requestOptions.Sort = normalizeSort(requestOptions.Sort)
options, err := store.ListOptionsFromMeta(requestOptions)
```

`meta.ListOption` 只表达请求默认值，`store.ListOption` 表达 Store 查询条件。
selector 转换错误在 HTTP seam 返回 BadRequest。请求和 Store 都按
`Limit>0`、`Size>0`、不分页的优先级选择行为；page 模式下 `Page<1` 按 1
处理，未选中模式的字段静默忽略。Client adapter 不选择或规范化分页行为，只把
非零 Store options 平铺到 HTTP query；接收端再按相同执行规则处理。没有分页值且
调用方未提供默认策略时，由拥有请求的服务决定不分页行为。

Update 不清除 ResourceVersion。请求对象携带非零版本时，底层 Store 按其乐观并发契约处理；只有调用方明确传入零值时才执行无条件更新。
