# Dynamic configuration design

`DynamicConfig` 是按 namespace 和 name 寻址的命名、版本化 JSON object 事实源。namespace 只是每次操作携带的逻辑 scope，不是需要管理生命周期的资源。根包只拥有配置契约，不依赖 Store、HTTP 或它们的元数据模型。

`Configuration` 只包含 Name、Version 和 Value。Value 使用 `map[string]any` 表达自然的 JSON object；所有 adapter 在写入边界统一拒绝 `null`、数组和标量，并使用 `json.Number` 解码数字。Version 0 表示尚未持久化，正数表示已提交版本。Get 缺失配置仍返回 Name、Version 0 和空 object，并将调用方的解码目标替换为对应零值。Set 无条件时 upsert；`IfVersion(0)` 表示仅在尚未持久化时写入，正版本执行 CAS。

Patch 只修改 Value，支持 JSON Merge Patch 和 JSON Patch。缺失配置以空 object 为基准，成功结果作为新配置持久化；补丁最终结果仍必须是 object。ListKeys 只列出已持久化配置的名称和版本，按名称稳定排序。接口不提供 Delete，也不暴露通用资源查询、分页或 Store 元数据。

Watch 是当前配置快照流，不是生命周期事件流。成功订阅后第一条消息始终是当前快照；缺失和删除都表示为 Version 0 的空 object，创建和更新表示为正版本快照。公共事件没有 initial、change 或 delete 类型。Store bookmark、HTTP event kind 和 checkpoint 都是 adapter 内部机制。`OnChange[T]` 为每条快照创建新的零值 T，并将对象和版本交给 callback。

`config/store`、`config/http` 和 `config/noop` 是同一 seam 的 adapter。Store adapter 独占持久化类型、namespace scope、原子 Patch 和 Store Watch 翻译；HTTP adapter 独占路径、ETag 条件与流编解码；Noop adapter 表示未启用配置中心。HTTP 服务端授权和公开读取策略属于宿主应用。
