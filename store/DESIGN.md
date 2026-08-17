# Store 实现契约

本文档面向 Store 实现方。调用方式由各业务模块的文档说明；这里定义所有直接存储实现必须遵守的行为。`store/storetest` 是这些规则的可执行版本，新实现必须接入并通过该套件。

## 基础契约

`Store` 的 CRUD、Count、精确 Scope、服务端元数据、Status 隔离、finalizer 删除流程和 `id` 排序属于基础能力，不能通过 `Capabilities` 关闭。

`Schema()` 返回 Store 构建时使用的资源定义快照。调用方可以读取或修改该快照，但修改不能影响 Store；`Scope()` 派生出的 Store 必须返回相同内容的独立快照。Schema 是本地契约，不通过 REST 动态推断，因此 REST Store 的构造方也必须显式提供 Schema。

Create 允许调用方不提供 ID。空 ID 生成 UUID，显式非空 ID 保留；UID、ResourceVersion、Generation、CreationTimestamp、Resource、Scopes 和 DeletionTimestamp 都由 Store 管理，调用方预填值不能覆盖服务端值。Generation 从 1 开始，ResourceVersion 在成功持久化后必须为正数。

Update 的 `ResourceVersion=0` 表示无条件更新，非零值在 `OptimisticLock` 开启时必须匹配当前版本，否则返回 Conflict。每次成功持久化都推进 ResourceVersion。ObjectMeta 和顶层 status 的变化不推进 Generation，其他业务字段发生变化时 Generation 加一。

普通 Update 和 Patch 保留当前 status。`Status().Update` 和 `Status().Patch` 只修改顶层 status，不能修改元数据、业务字段或 Generation。Patch 必须基于存储中的当前对象原子应用，传入对象上的 ResourceVersion 不是 Patch 的隐式前置条件。

Merge Patch 未携带 ResourceVersion 或携带零值时是无条件 Patch；携带非零 ResourceVersion 时必须匹配当前对象，否则返回 Conflict。仓库的 ObjectMeta 使用扁平 JSON，因此条件字段是顶层 `resourceVersion`。JSON Patch 可以通过 `test /resourceVersion` 表达条件，test 失败返回 HTTP 422 且后续操作不执行。条件验证完成后，ResourceVersion 仍由 Store 管理并在成功写入时推进。

## 删除与 Scope

Delete 默认使用 Background。没有 finalizer 时立即物理删除；存在 finalizer 时设置一次不可变的 DeletionTimestamp，对象继续可读。Foreground 和 Orphan 分别添加 `foregroundDeletion` 和 `orphan` finalizer。对象进入删除状态后，普通 Update 或 Patch 清空最后一个 finalizer 必须自动完成物理删除。

单对象 Delete 支持两类用途不同的条件。`Preconditions.UID` 和 `Preconditions.ResourceVersion` 是对象身份与版本的并发前置条件；UID 防止同 ID 重建后的新对象被旧请求误删，ResourceVersion 保证请求只作用于调用方指定的对象版本。`LabelRequirements` 和 `FieldRequirements` 是当前对象的选择条件，可表达“仅当对象仍处于某个业务状态时删除”。单对象 Delete 由 ID 定位至多一个对象，再对该对象应用 requirements；DeleteBatch 不提供逐对象 UID 或 ResourceVersion precondition，而是按 requirements 选择并删除零到多个对象，因此两者不能互相替代。

接口使用一个与 Kubernetes 同义的强类型前置条件，并保留单对象 requirements：

```go
type Preconditions struct {
    UID             *string
    ResourceVersion *int64
}

type DeleteOptions struct {
    LabelRequirements  Requirements
    FieldRequirements  Requirements
    Preconditions      *Preconditions
    PropagationPolicy  *DeletionPropagation
    DryRun             bool
}
```

`WithDeleteUID` 和 `WithDeleteResourceVersion` 分别设置对应 precondition，并且可以与 `WithDeleteLabelRequirements`、`WithDeleteFieldRequirements` 组合。

Delete 按以下顺序判断，并使用固定的错误语义：精确 Scope 和 ID 下没有对象时返回 NotFound；对象存在但任一 UID 或 ResourceVersion precondition 不匹配时返回 Conflict；preconditions 匹配但任一 Label 或 Field requirement 不匹配时返回 NotFound。两类条件同时提供时使用 AND 关系。只有 `DeleteOptions.Preconditions` 中的值构成并发前置条件，传入对象自身的 UID 和 ResourceVersion 不产生隐式条件。nil 表示未提供对应 precondition；非 nil 值必须精确匹配，零值也不表示无条件。

条件判断与 Delete 的最终写入必须是一个原子操作。该写入既包括物理删除，也包括设置 DeletionTimestamp 或添加传播 finalizer；任何条件失败都不能修改存储对象、推进 ResourceVersion 或产生 Watch 事件。实现可以在事务语句中同时表达 ID、Scope、requirements、preconditions 和当前存储版本，也可以读取当前对象后以存储版本执行 CAS。CAS 失败时必须基于最新对象重新判断调用方最初提供的全部条件并重新计算删除结果：precondition 已不匹配则返回 Conflict，requirement 已不匹配则返回 NotFound，条件仍匹配才可继续尝试。重试由请求 context 和确定的条件控制，不能使用固定次数、sleep 或时间窗口猜测正确性。

调用方 precondition 与实现内部 CAS 版本是两个概念。前者在整个请求期间固定，表达调用方允许删除的身份和版本；后者只保护一次实际写入，可以在实现重新读取后变化。所有 Store adapter 都必须支持 UID 和 ResourceVersion preconditions，不能通过 Capabilities 关闭。adapter 不支持某类 Label 或 Field requirement 时必须在写入前明确返回 Unsupported，不能忽略条件或退化为仅按 ID 删除。

REST Delete 不使用 request body，所有选项都通过 query 参数传递，避免依赖客户端、代理和网关对 DELETE body 的支持。单对象 Delete 使用 `uid`、`resourceVersion`、`propagationPolicy`、`dryRun`、`labelSelector` 和 `fieldSelector`；参数是否出现决定对应 pointer 是否为 nil，因此显式的空 UID 或零 ResourceVersion 仍是需要精确匹配的 precondition，不能按未提供处理。HTTP adapter 将参数一次性转换为 DeleteOptions 后只调用一次底层 Delete，Store 的领域类型不携带 HTTP 或 JSON tag。Conflict 映射为 HTTP 409，requirement 不匹配和对象不存在映射为 HTTP 404。DeleteBatch 继续使用 `dryRun`、`labelSelector` 和 `fieldSelector`，不接受单个对象的 UID 或 ResourceVersion precondition。

Store 只管理传播 finalizer，不遍历 OwnerReferences，也不递归删除依赖对象；依赖传播由 garbage collector 负责。

Scope 是有序层级。默认 Get、List、Count 和写操作只匹配完整的当前 Scope，不得命中兄弟或祖先 Scope。`IncludeSubScopes` 只包含当前 Scope 的后代，并且只有声明 `SubScopes` 的实现才可使用。

## 查询与能力

`Capabilities()` 只描述实现支持的可选行为，供调用方发现能力，也供契约测试选择需要验证的正向语义。它不是运行时校验开关：Store 方法不能根据自身的能力声明决定是否接受某个 option。实现需要拒绝不支持的输入时，应根据自身约束直接返回错误。

List 在未指定排序时不保证顺序。所有实现必须支持 `id+` 和 `id-`。其他排序字段必须来自 ResourceSchema 中某个 Index 的字段前缀；复合排序必须全部升序或全部降序。Search 是大小写不敏感的子串匹配，未指定 SearchFields 时搜索 id 和 name，多字段采用任一字段匹配；显式 SearchFields 覆盖默认字段。

`Page>0` 且 `Size>0` 使用页码分页。`Page=0` 且 `Size>0` 或 Continue 非空时使用 continuation 分页；Continue 是实现方生成的不透明 token。Page、Continue 和 ContinueWithSort 分别声明能力。Page、Size 和 Continue 都为空时是不分页查询。

Labels 和 Annotations 必须无损保存，包括含 `.`, `/` 或 `$` 的键。声明 `LabelSelector` 的实现必须直接按 Requirement 的键匹配，不能因底层数据库路径语法或 Kubernetes label parser 改变键的含义。Annotations 不自动成为 label selector。

声明 DryRun 后，Create、Update、Patch、Delete、Batch 和 Status 写入都必须完成正常校验并返回模拟结果，但不能落库、产生 Watch 事件、创建 TTL 或修改索引。

## Watch 与缓存

`Capabilities.Watch=true` 表示实现提供足以构建严格缓存的完整 Watch 行为，而不表示它一定支持历史续传。Watch 成功返回后订阅必须已经生效；事件必须有序且不能静默丢失。Stop 必须幂等，调用 Stop 或取消创建 Watch 的 context 后 Events channel 最终关闭；运行期失败先发送一个 terminal error event，再关闭 channel。

缓存调用方统一使用 initial Watch，不自行组合普通 `List` 和 `Watch`：

```go
watcher, err := storage.Watch(ctx, list, store.WithSendInitialEvents())
```

调用方把第一个 Bookmark 前的所有对象事件应用到 staging state；收到第一个 Bookmark 后，staging state 已是 authoritative snapshot，此时原子替换 active state。Bookmark 后的事件直接应用到 active state。空快照也必须发送 Bookmark。连接中断时，最简单且通用的恢复方式是不传 ResourceVersion，重新启动一次 initial Watch；未收到 Bookmark 的 staging state 必须丢弃。调用方不能把普通 `List` 返回的对象 ResourceVersion 最大值当作 Watch 位置。

实现可以采用不同机制满足同一个 initial Watch 契约：etcd 在版本 R 上 List 后从 R+1 Watch；支持原生 watch-list 的后端直接转发该语义；内存实现可在同一临界区注册 Watch 并取得快照；MongoDB 必须先建立 Change Stream，再 Find，并在 Bookmark 前发送 Find 期间已积累的变更。首个 Bookmark 只能在这些变更已纳入 staging state 后发送，不能仅表示 Find 已结束。

Delete 必须携带删除前的完整对象。Update 需要同时判断旧对象和新对象是否匹配 ID、Scope、Label 和 Field selector，并按以下规则转换事件：旧状态不匹配而新状态匹配为 Create；两者都匹配为 Update；旧状态匹配而新状态不匹配为 Delete；两者都不匹配则不发送事件。

Watch 不区分能力层级。`WatchEvent.ResourceVersion` 是可选的全局 Watch checkpoint，不是 `Object.ResourceVersion`。实现能够公开恢复位置时发送正数 checkpoint；调用方可以通过 `WithWatchResourceVersion(R)` 请求其后的全部事件。实现没有历史、历史已过期或无法识别该位置时都返回 ResourceExpired，调用方转为新的 initial Watch。全局 Watch ResourceVersion 只是可选的断线恢复优化，不是缓存正确性的前提。

MongoDB 声明 `Watch=true`。它以 `batchSize=0` 建立 Change Stream，并在 Find 前消费明确为空的 initial batch。Initial Find 使用 snapshot read concern，在单一时间点取得一致快照并产生 synthetic Create；Find 后的第一次 TryNext 因当前批次已耗尽，按 Go driver 的公开契约必然执行一次 getMore。实现处理该 getMore 的完整批次后才发送 Bookmark，因此切换点由一致快照和服务端 post-batch resume token 共同确定，而不是由轮询次数、延时或缓冲区状态猜测。调用方通过 UID 和对象 ResourceVersion 合并快照与该批次的重叠状态。MongoDB 不提供 Store 的整数全局 checkpoint，传入正数 WithWatchResourceVersion 时返回 ResourceExpired。

## 实现与测试

直接实现通过 `store/storetest.Fixture` 提供隔离 Store 和预期 Capabilities。套件首先比较 Store 的能力声明，再验证基础契约和声明为支持的能力；未声明的能力不参与测试。

MongoDB、MySQL 和 PostgreSQL 套件使用顶层 `testkit` 启动或连接真实数据库；etcd 和 etcdcache 使用隔离前缀。实现特有的编码、索引、事务和 Watch 细节继续保留在各实现自己的测试中。

业务编号不属于 Store ID。需要仓库内 issue number 等单调序号时使用独立的 `sequence.Allocator`；对象 ID 仍使用 UUID。序号按调用方提供的稳定 key 隔离，保证单调且唯一，但允许空洞，也不保证与业务对象 Create 处于同一事务。
