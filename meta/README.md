# Meta 使用指南

`meta` 提供跨 HTTP、客户端和 Store 共享的对象元数据、列表请求与列表响应类型。
它定义可传递的数据契约，不负责解析具体传输协议，也不实现某个存储后端的查询。
分页的实现约束和设计理由见 [DESIGN.md](./DESIGN.md)。

## 列表与分页

列表请求使用平铺的 `meta.ListOptions`：

```go
options := meta.ListOptions{
    Page: 2,
    Size: 20,
    Sort: "name-",
}
```

分页没有 `mode` 字段，也不嵌套第二层对象。执行列表的服务按固定优先级选择行为：

| 条件 | 行为 |
| --- | --- |
| `Limit > 0` | continuation；`Continue` 是可选的后续游标 |
| 否则 `Size > 0` | page；`Page < 1` 按第一页处理 |
| 否则 | 不分页，由服务决定具体查询方式 |

选中一种行为后，其他分页字段静默忽略。混合、不完整和负值字段不构成需要校验的
分页组合。实现不支持被选中的行为时，可以返回自身定义的 Unsupported 错误。

服务可以使用具体的默认 option：

```go
defaults := []meta.ListOption{
    meta.DefaultPage(1, 20),
    meta.DefaultSort("creationTimestamp-"),
}
```

`DefaultPage` 在没有非空 `Continue` 或正数 `Limit` 表达 continuation 意图时分别
填充零值 `Page` 和 `Size`；`DefaultContinuation` 在 `Limit` 为零且没有非零
`Page` 或正数 `Size` 表达 page 意图时填充 `Limit`。两者都不会用默认值覆盖另一
组显式分页字段。`DefaultPage(1, size)` 明确将第一页作为默认页码；
`DefaultPage(0, size)` 保持页码未指定，并在正数 `Size` 选择 page 行为后由执行层
归一为第一页。`DefaultSort` 只填充空 `Sort`。请求 adapter 应先解析请求值，再
按声明顺序应用这些 option，使默认项填充解析后的零值。若同时配置 page 和
continuation 默认值，声明在前的分页默认值选择行为。`ApplyListOptions` 用于从零值
开始按声明顺序展开一组 option。

## 列表响应

列表响应使用平铺的 `meta.Page[T]`，客户端通过出现的字段识别服务实际返回的行为：

| 响应行为 | 包含 | 省略 |
| --- | --- | --- |
| Page | `items`, `page`, `size`, `total` | `continue`, `limit` |
| Continuation | `items`, `limit`，存在下一批时包含 `continue` | `page`, `size`, `total` |
| 不分页 | `items`, `total` | `page`, `size`, `continue`, `limit` |

Continuation 的终止响应仍保留 `limit`；省略或返回空 `continue` 都表示遍历完成。
`Total` 使用 `*int`，因此精确总数 `0` 与不提供总数可以区分。集合级
`ResourceVersion` 描述本次列表快照，与每个对象自身的版本不同。

使用 `ConvertPage` 转换 item 类型时，分页字段和集合级 `ResourceVersion` 会原样
保留：

```go
result := meta.ConvertPage(page, func(item StoredUser) User {
    return item.User
})
```

## 查询与对象元数据

`Search`、`Sort`、`FieldSelector` 和 `LabelSelector` 只是共享请求表示；具体支持的
字段、排序和 selector 能力由执行服务声明。`ParseSearch` 和 `ParseSort` 提供共享
字符串解析，不替服务决定查询能力。

`ObjectMetadata` 提供对象 ID、显示名称、时间、标签、注解和并发版本等公共字段。
`Time`、`Duration`、`Ptr`、`DerefPtr`、`Map` 和 `Reduce` 是相邻的轻量值辅助能力。
