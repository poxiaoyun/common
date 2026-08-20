# Meta 设计契约

`meta` 是跨协议和存储实现共享的数据契约 owner。它拥有字段名称、序列化形状和
与具体后端无关的选择语义；HTTP query 解析属于 `rest/api`，Store option 转换和
后端能力属于 `store`，客户端遍历属于 `httpclient`。

## 分页请求契约

`ListOptions` 使用四个平铺字段表达两种分页方式：

- `Page` 和 `Size` 表达页码分页。
- `Continue` 和 `Limit` 表达 continuation 分页。

不提供 `mode`，因为执行行为可以由数值本身确定，响应也能通过保留的字段确定实际
行为。所有消费者使用同一优先级：

1. `Limit > 0` 选择 continuation，`Continue` 为空表示首批。
2. 否则 `Size > 0` 选择 page，`Page < 1` 规范为 `1`。
3. 否则执行不分页行为。

未选中行为的字段静默忽略。`ListOptions` 不提供分页 validator；请求 adapter、内存
helper 和 Store adapter 不应分别发明冲突规则或重复校验。后端只在被选中的能力
本身不受支持时返回错误。

`ListOption` 是开放的请求 option seam。具体 option 通过
`ApplyToList(*ListOptions)` 按声明顺序应用：

- `DefaultPageOption` 在 `Continue` 为空且 `Limit` 不为正数时分别填充零值
  `Page` 和 `Size`，防止默认 page 覆盖显式 continuation 意图。默认 `Page=1`
  明确选择第一页；默认 `Page=0` 保持页码未指定，由正数 `Size` 选择 page 行为后
  在执行层归一为第一页。
- `DefaultContinuationOption` 在 `Limit` 为零、`Page` 为零且 `Size` 不为正数时
  填充 `Limit`，防止默认 continuation 覆盖显式 page 意图。
- `DefaultSortOption` 只填充空 `Sort`。

默认 option 只写入自己拥有的字段；两个分页默认 option 分别读取另一组的意图字段，
仅用于保留显式请求，不验证分页组合。HTTP adapter 先解析 query，再
应用 option；`ApplyListOptions` 则从零值 accumulator 开始展开 option。若同时
声明两种分页默认值，声明顺序决定默认行为。两者都不验证或规范化分页，最终行为
选择留给执行列表的模块。

## 分页响应契约

`Page[T]` 与请求一样保持平铺。响应生产者只设置实际行为所属的字段：

- Page 响应设置 `Page`、`Size` 和非 nil `Total`。
- Continuation 响应设置正数 `Limit`，仅在存在下一批时设置 `Continue`，并保持
  `Total == nil`。
- 不分页响应只设置非 nil `Total`。

`Total` 必须是指针，因为 `0` 是有效精确总数，而 nil 表示当前响应行为不提供总数。
Continuation 终止时仍保留 `Limit`，所以客户端不需要 `mode`，也不需要把空 token
与未分页响应混为一谈。响应消费方信任生产者提供的字段，不重复验证返回值。

`ResourceVersion` 是可选的集合快照标识。它不能从 item 版本推断，也不能与对象
自身的 `resourceVersion` 混用。无法表达共享整数快照版本的实现应省略它。

`ConvertPage` 只转换 `Items`，并保留全部分页元数据和集合级
`ResourceVersion`。新增响应元数据时必须同步更新转换逻辑和 JSON 契约测试。

## 依赖方向

协议和实现层可以依赖 `meta`，但 `meta` 不依赖它们：

```text
rest/api ─┐
store ────┼──> meta
httpclient┘
```

因此以下行为不属于 `meta`：

- query 参数是否存在以及如何解析；
- continuation token 的编码、快照和过期规则；
- Store 是否支持 page、continuation、排序或 selector；
- 多页客户端何时发起下一次网络请求。

这些模块必须复用本文定义的字段含义和选择优先级，但各自拥有自己的传输或执行
机制。
