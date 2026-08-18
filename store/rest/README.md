# REST Store

`store/rest` 提供 `store.Store` 的 HTTP adapter。

- `NewServer` 和 `NewRemoteStore` 暴露并访问通用 Store HTTP 协议。
- 对象级 List、Watch、Get、Create、Patch、Update 和 Delete 函数用于已有领域路由，接受调用方创建的具体 `store.Object` 或 `store.ObjectList`。
- `ListOptionsFromRequest` 将列表 query 转换为完整的 `store.ListOptions`。

对象级 List/Watch 会先应用请求 query，再应用调用方传入的 `store.ListOption`。因此调用方可以添加不可由客户端移除的 selector，并可显式设置服务端分页或排序策略。无效 selector 或 resourceVersion 返回 BadRequest。

Update 不清除 ResourceVersion。请求对象携带非零版本时，底层 Store 按其乐观并发契约处理；只有调用方明确传入零值时才执行无条件更新。
