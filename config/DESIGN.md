# Dynamic configuration design

`DynamicConfig` 是按 namespace 和 name 寻址的命名、版本化 JSON 文档事实源。namespace 是稳定的逻辑 Store scope，不是需要注册、查询或管理生命周期的资源。namespace 是每次操作的参数而不是 adapter 的构造参数，因此同一个客户端可以访问多个 namespace。

`config` 根包只拥有契约：`DynamicConfig`、`Configuration`、写入条件、Watch 事件以及 `OnChange`。它不创建 Store 或 HTTP 客户端，也不注册持久化 schema。`Configuration` 保留任意合法 JSON，并继续作为各 adapter 共享的结果类型；不引入第二个 `StoredConfiguration` 模型。

调用接口面向 Go 对象而不是 `json.RawMessage`：Set 在内部序列化对象，Get 和 Patch 将持久化结果反序列化到调用方传入的对象。Get 找不到配置时返回 nil Configuration 和 nil error，并保持目标对象不变，使可选配置天然等价于空值；存在的 JSON `null` 仍返回非 nil Configuration。Set 无 option 时执行 upsert，`IfAbsent` 仅创建，`IfVersion` 执行 CAS。Patch 只修改 Value，支持 JSON Merge Patch 和 JSON Patch；无条件补丁由 Store 原子应用，带版本的 JSON Patch 先计算目标值再以同一版本执行 CAS，从而同时保留用户 `test` 的语义错误和版本冲突。元数据不能成为补丁目标。接口不提供 List 和 Delete。

Watch 与 Get 一样按 namespace 和 name 观察一个已知配置，只返回基础 Watcher。每次 Watch 的第一条事件固定是 Initial：Configuration 非 nil 表示当前值，nil 表示配置不存在；之后只传播 Change、Delete 或终止错误。Store Bookmark 和流 checkpoint 是 adapter 内部机制，不进入公共接口。泛型 `OnChange[T]` 封装 Watch 生命周期、对象分配、JSON 解码和 callback；每个值事件都创建新的 `T`，删除和 initial missing 以 nil Object 回调，避免共享可变对象。

`config/store`、`config/http` 和 `config/noop` 是同一 `DynamicConfig` seam 的 adapter。Store adapter 独占 schema 注册、`namespaces/{namespace}` scope、原子 Patch 和 Store Watch 翻译；资源名按 Store 默认规则由 `Configuration` 类型推导为 `configurations`，不覆盖 `ResourceName`。HTTP adapter 独占 namespace 路径、Bearer Token、ETag 条件和 SSE 编解码。Noop adapter 表示配置中心未启用：读取永远 missing，Watch 只保持 missing initial 状态，写入明确不支持。HTTP 服务器和公开策略属于宿主应用，不属于这些 adapter。

可执行程序启动配置仍由 `command.Source` 所有。`config/commandsource` 拥有统一的 Configcenter `Options{Address, Token}` 和 adapter 选择：Address 默认为空并选择 Noop；非空且没有 scheme 时默认补充 `http`；`http` 与 `https` 交给 HTTP adapter，其他 scheme 在拥有实际实现后加入显式分派。它通过 `DefaultSources` 将配置中心 Source 放在配置文件之后、环境变量与命令行之前，并声明 `configcenter-address` 与 `configcenter-token` 控制参数。它也可以接受 composition root 注入的现成 `DynamicConfig`。根契约不导入任何 adapter，避免依赖反转和循环。
