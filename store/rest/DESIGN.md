# REST Store 设计

`store/rest` 是 HTTP 与 `store.Store` 之间的适配 seam。`store` 根包保持与传输协议无关；HTTP query、body、状态响应和流式 Watch 都由本包处理。

本包维护两种 adapter：`Server` 和 `Client` 实现可远程访问的通用 Store 协议；对象级函数供具有自身路由和领域类型的 HTTP 接口复用 Store CRUD 与 Watch 行为。两者共享 Store option 和对象语义，但各自拥有请求参数边界。

对象级列表先解析平铺的分页值，再按声明顺序应用调用方提供的
`meta.ListOption`；默认 option 只写入自己的字段，且两种分页默认值都不覆盖另一组
非空字段表达的显式意图。分页选择统一遵循
`Limit>0`、`Size>0`、不分页的优先级，`Page<1` 在 page 模式下按 1 处理，其他
模式的字段静默忽略。随后由根包
`store.ListOptionsFromMeta` 转换为只包含公开列表字段的具体 `store.ListOption`。
转换结果不包含完整 `store.ListOptions` 替换 option，也不使用捕获闭包。
`ListOptionsFromRequest` 只拥有上述请求解析和默认值转换；调用方在返回结果后追加
`store.ListOption`，增加业务约束或覆盖标量字段。Store
HTTP Server 在自身的协议边界单独解析 resourceVersion、includeSubscopes 和 fields；
业务 REST helper 不解析这些 Store 协议字段。

Client adapter 不判断 Store options 的分页行为或合法性，也不规范化页码。它将
非零 `Page`、`Size`、`Continue` 和 `Limit` 原样序列化到 HTTP query，由接收端
Store 按自身执行规则选择行为。

List 与 Watch 使用同一组 selector、scope 和 resourceVersion。分页、排序、搜索、continuation 和返回字段只作用于 List，不进入 Watch。Update 保留请求对象携带的 ResourceVersion，使 Store 执行正常的乐观并发控制。
