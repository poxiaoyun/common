# REST Store 设计

`store/rest` 是 HTTP 与 `store.Store` 之间的适配 seam。`store` 根包保持与传输协议无关；HTTP query、body、状态响应和流式 Watch 都由本包处理。

本包维护两种 adapter：`Server` 和 `Client` 实现可远程访问的通用 Store 协议；对象级函数供具有自身路由和领域类型的 HTTP 接口复用 Store CRUD 与 Watch 行为。两者共享请求参数到 Store options 的转换，但保留各自的路由和响应表示。

列表请求先由 `rest/api` 解码为 `meta.ListOptions`，再由 `store` 转换为规范的 `store.ListOptions`。HTTP 特有的 resourceVersion、includeSubscopes 和 fields 在本包补充。调用方提供的额外 `store.ListOption` 最后应用，因此可以增加服务端约束或覆盖标量选项。

List 与 Watch 使用同一组 selector、scope 和 resourceVersion。分页、排序、搜索、continuation 和返回字段只作用于 List，不进入 Watch。Update 保留请求对象携带的 ResourceVersion，使 Store 执行正常的乐观并发控制。
