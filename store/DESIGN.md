# Store 实现契约

本文档面向 Store 实现方。调用方式由各业务模块的文档说明；这里定义所有直接存储实现必须遵守的行为。`store/storetest` 是这些规则的可执行版本，新实现必须接入并通过该套件。

## 基础契约

`Store` 的 CRUD、Count、精确 Scope、服务端元数据、Status 隔离、finalizer 删除流程和 `id` 排序属于基础能力，不能通过 `Capabilities` 关闭。

`Schema()` 返回 Store 构建时使用的资源定义快照。调用方可以读取或修改该快照，但修改不能影响 Store；`Scope()` 派生出的 Store 必须返回相同内容的独立快照。Schema 是本地契约，不通过 REST 动态推断，因此 REST Store 的构造方也必须显式提供 Schema。

Create 允许调用方不提供 ID。空 ID 生成 UUID，显式非空 ID 保留；UID、ResourceVersion、Generation、CreationTimestamp、Resource、Scopes 和 DeletionTimestamp 都由 Store 管理，调用方预填值不能覆盖服务端值。Generation 从 1 开始，ResourceVersion 在成功持久化后必须为正数。

资源名默认由对象类型名按统一复数规则推导。对象只在既有持久化或外部协议名称与该规则不一致、且兼容该名称是当前要求时实现 `ResourceName() string`；不得为了显式重复默认结果而实现它。

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

`WithUID` 和 `WithResourceVersion` 分别设置对应 precondition，并且可以与 `WithLabelRequirements`、`WithFieldRequirements` 组合。

`WithPreconditions` 直接接受 Store 的 `Preconditions`；协议边界负责按字段是否出现构造对应指针。`PreconditionsOption` 只覆盖自身非空字段，不清除先前设置的其他条件，因此非 nil 的零值仍是需要精确匹配的有效条件。

Delete 按以下顺序判断，并使用固定的错误语义：精确 Scope 和 ID 下没有对象时返回 NotFound；对象存在但任一 UID 或 ResourceVersion precondition 不匹配时返回 Conflict；preconditions 匹配但任一 Label 或 Field requirement 不匹配时返回 NotFound。两类条件同时提供时使用 AND 关系。只有 `DeleteOptions.Preconditions` 中的值构成并发前置条件，传入对象自身的 UID 和 ResourceVersion 不产生隐式条件。nil 表示未提供对应 precondition；非 nil 值必须精确匹配，零值也不表示无条件。

条件判断与 Delete 的最终写入必须是一个原子操作。该写入既包括物理删除，也包括设置 DeletionTimestamp 或添加传播 finalizer；任何条件失败都不能修改存储对象、推进 ResourceVersion 或产生 Watch 事件。实现可以在事务语句中同时表达 ID、Scope、requirements、preconditions 和当前存储版本，也可以读取当前对象后以存储版本执行 CAS。CAS 失败时必须基于最新对象重新判断调用方最初提供的全部条件并重新计算删除结果：precondition 已不匹配则返回 Conflict，requirement 已不匹配则返回 NotFound，条件仍匹配才可继续尝试。重试由请求 context 和确定的条件控制，不能使用固定次数、sleep 或时间窗口猜测正确性。

调用方 precondition 与实现内部 CAS 版本是两个概念。前者在整个请求期间固定，表达调用方允许删除的身份和版本；后者只保护一次实际写入，可以在实现重新读取后变化。所有 Store adapter 都必须支持 UID 和 ResourceVersion preconditions，不能通过 Capabilities 关闭。adapter 不支持某类 Label 或 Field requirement 时必须在写入前明确返回 Unsupported，不能忽略条件或退化为仅按 ID 删除。

REST Delete 不使用 request body，所有选项都通过 query 参数传递，避免依赖客户端、代理和网关对 DELETE body 的支持。单对象 Delete 使用 `uid`、`resourceVersion`、`propagationPolicy`、`dryRun`、`labelSelector` 和 `fieldSelector`；参数是否出现决定对应 pointer 是否为 nil，因此显式的空 UID 或零 ResourceVersion 仍是需要精确匹配的 precondition，不能按未提供处理。HTTP adapter 将参数一次性转换为 DeleteOptions 后只调用一次底层 Delete，Store 的领域类型不携带 HTTP 或 JSON tag。Conflict 映射为 HTTP 409，requirement 不匹配和对象不存在映射为 HTTP 404。DeleteBatch 继续使用 `dryRun`、`labelSelector` 和 `fieldSelector`，不接受单个对象的 UID 或 ResourceVersion precondition。

Store 只管理传播 finalizer，不遍历 OwnerReferences，也不递归删除依赖对象；依赖传播由 garbage collector 负责。

Scope 是有序层级。默认 Get、List、Count 和写操作只匹配完整的当前 Scope，不得命中兄弟或祖先 Scope。`IncludeSubScopes` 只包含当前 Scope 的后代，并且只有声明 `SubScopes` 的实现才可使用。

## 查询与能力

`PageFromList` 将 Store 的列表结果投影为 `meta.Page`，不暴露 Store 的
resource 和 scope 元数据。

`Capabilities()` 只描述实现支持的可选行为，供调用方发现能力，也供契约测试选择需要验证的正向语义。它不是运行时校验开关：Store 方法不能根据自身的能力声明决定是否接受某个 option。实现需要拒绝不支持的输入时，应根据自身约束直接返回错误。

List 在未指定排序时不保证顺序。所有实现必须支持 `id+` 和 `id-`。其他排序字段必须来自 ResourceSchema 中某个 Index 的字段前缀；复合排序必须全部升序或全部降序。Search 是大小写不敏感的子串匹配，未指定 SearchFields 时搜索 id 和 name，多字段采用任一字段匹配；显式 SearchFields 覆盖默认字段。

List 使用平铺的 `Page`、`Size`、`Continue` 和 `Limit` 表达分页请求，不提供
mode 字段，也不复用一个字段表达两种分页含义。字段含义以
[meta 分页契约](../meta/DESIGN.md#分页请求契约)为准。请求解析和 Store 执行使用相同的
分页选择优先级，不把混合、不完整或负值字段定义为非法组合。

Store 实现按固定优先级选择行为：`Limit>0` 使用 continuation，`Continue` 为空
表示首批；否则 `Size>0` 使用 page，`Page<1` 按第一页处理；否则不分页并返回
全部匹配对象。选中一种行为后静默忽略另一组及不完整字段。Store 自身不选择默认
分页方式；公开请求没有分页参数时，拥有该请求契约的服务可以在调用 Store 前选择
页码分页、continuation 分页或不分页。Page、Continue 和 ContinueWithSort 分别
声明能力；实现不支持选中的分页方式时返回 Unsupported。

List 响应同样平铺分页字段，并只序列化当前方式支持的元数据。页码分页返回
`Page`、`Size` 和精确的 `Total`，省略 `Continue` 和 `Limit`；continuation
分页返回有效 `Limit` 以及存在下一批时的 `Continue`，省略 `Page`、`Size` 和
`Total`；不分页返回精确的 `Total`，省略四个分页字段。空的 `Continue` 表示
continuation 遍历结束，`Limit` 仍保留，因此客户端不需要 mode 字段推断当前
响应方式。`Total=0` 是页码或不分页查询的有效结果，因此 Total 必须能区分零值
与不支持。Continue 是实现生成的不透明 token。

Store 操作使用 variadic option interface。每个 option 是实现
`ApplyToXxx(*XxxOptions)` 的具体值；标准 option 不得使用捕获闭包。
`XxxOptions` 只表示已解析的最终值，不实现 option interface。完整 Options
作为 option 时，整体替换、仅覆盖有值字段和合并集合三种行为都可能合理，
语义并不明确，因此不提供完整 Options option。标量具体 option 按顺序覆盖，
requirements 具体 option 按顺序追加。

同一领域语义跨多个 Store 操作时只保留一个 `WithXxx` 入口，由同一个具体
option 类型实现所需的多个 `ApplyToXxx`。Option 类型和构造函数按字段或行为
命名，不重复目标操作，例如 `WithID`、`WithTimeout` 和 `WithPreconditions`。

`ApplyXxxOptions` 是 option slice 到最终 `XxxOptions` 的唯一展开入口。
各 Store adapter 调用该入口，不重复实现 option 循环。`ApplyListOptions` 只负责
展开可信 option，不验证分页、不返回错误；adapter 按上述固定优先级执行。每个
入口在同一函数内对空 option slice 直接返回零值，避免 interface 展开使
accumulator 逃逸；非空 slice 仍在该函数中线性展开，不拆出第二套 helper。

`ListOptionsFromMeta` 是公开列表契约到 `[]ListOption` 的唯一转换入口。
它将 Page、Size、Continue、Limit、Search、Sort 和已解析 selector 表示为一个只含
公开列表字段的具体 option，再追加调用方 option。它不解析
Store HTTP 协议字段，也不选择默认分页模式。边界默认值在转换前通过
`meta.ListOption` 应用，使请求默认值留在公开请求契约中；业务和协议约束
在转换后作为 Store option 追加。

Labels 和 Annotations 必须无损保存，包括含 `.`, `/` 或 `$` 的键。声明 `LabelSelector` 的实现必须直接按 Requirement 的键匹配，不能因底层数据库路径语法或 Kubernetes label parser 改变键的含义。Annotations 不自动成为 label selector。

`selector.Requirement` 是递归的共享布尔选择条件。Store 保留
`store.Requirements = selector.Requirements` alias 作为 Store API 和各 backend
直接使用的集合类型；selector 仍是表达式语义的唯一 owner。顶层 Requirements 按
AND 组合，其中每个 Requirement 可以是常量、组合节点或一个键上的叶子条件：

```go
type Requirement struct {
    Operator     Operator
    Key          string
    Values       []any
    Requirements Requirements
}
```

有效字段形状如下：

| Operator | Key | Values | Requirements |
| --- | --- | --- | --- |
| `None`, `All` | empty | empty | empty |
| `And`, `Or` | empty | empty | zero or more children |
| `Not` | empty | empty | exactly one child |
| `Exists`, `DoesNotExist` | non-empty | empty | empty |
| single-value comparisons and `Like` | non-empty | exactly one value | empty |
| `In`, `NotIn`, `Contains` | non-empty | zero or more values | empty |

零值 Requirement 是 `None`，不匹配任何对象。空的顶层 `Requirements` 和空的
`And` 匹配全部对象，空的 `Or` 不匹配任何对象。`Not` 对完整的唯一子表达式取反。
缺失键只满足 `DoesNotExist`、`NotEquals` 和 `NotIn`；组合操作在叶子结果上继续
求值。集合操作符允许空 Values；空集合条件不匹配对象。

每个 Store 操作在读取或修改数据前验证 Label 和 Field Requirements。无效字段
形状和未知 Operator 返回 BadRequest。adapter 无法无损执行某个有效操作符或组合
时，必须在产生部分结果或效果前返回 Unsupported。Requirements 必须在排序、分页、
Count 和修改前执行；Watch 使用同一个表达式分别判断旧对象和新对象，再生成成员关系
转换事件。

`LabelSelector` 和 `FieldSelector` capability 分别表示支持对应键空间，并能对
adapter 已支持的叶子操作符执行 `None`、`All`、`And`、`Or`、`Not` 组合。合法但
底层无法无损执行的叶子操作符仍返回 Unsupported。只支持更窄外部 selector 协议的
Store 不得声明相应 capability，并且必须拒绝无法表示的 Requirement，不能弱化
条件。Label Requirements 和 Field Requirements 仍是两个最后用 AND 连接的表达式；
一个组合节点不能混合 label 键和 field 键。

Requirement 的规范文本编码由 `selector` 拥有，并扩展 Kubernetes selector 的可读语法。
顶层逗号仍表示 AND；组合节点使用 `&&`、`||`、`!(...)` 和括号；常量写作
`all()` 与 `none()`。叶子沿用 `key=value`、`key in (a,b)`、`!key` 等 selector
形式，并增加 Store 已有的比较和包含操作符。`Requirements.String` 生成规范文本，
`ParseRequirements` 解析同一语法；常用的平铺 Kubernetes label selector 保持相同
写法。

值默认使用 selector 风格的裸文本。包含语法分隔符或空白的字符串使用双引号和 Go
字符串转义；字符串 `"null"` 也加引号，以区别 nil 的 `null` 字面量。除 nil 外，协议
解析出的 operand 都是字符串，与 Kubernetes selector 一致；Requirement 求值层负责
与字段中的布尔值、数值和时间比较。因此具体 Go 数值宽度不是协议语义。Requirement
值只接受 nil、字符串、布尔值、整数、有限浮点数和时间；其他值在 Store 或协议执行前
验证失败。HTTP adapter 只传输该文本，不维护第二套 Requirement wire model。

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

Watch 不区分能力层级。`WatchEvent.ResourceVersion` 是可选的全局 Watch checkpoint，不是 `Object.ResourceVersion`。实现能够公开恢复位置时发送正数 checkpoint；调用方可以通过 `WithResourceVersion(R)` 请求其后的全部事件。实现没有历史、历史已过期或无法识别该位置时都返回 ResourceExpired，调用方转为新的 initial Watch。全局 Watch ResourceVersion 只是可选的断线恢复优化，不是缓存正确性的前提。

MongoDB 声明 `Watch=true`。它以 `batchSize=0` 建立 Change Stream，并在 Find 前消费明确为空的 initial batch。Initial Find 使用 snapshot read concern，在单一时间点取得一致快照并产生 synthetic Create；Find 后的第一次 TryNext 因当前批次已耗尽，按 Go driver 的公开契约必然执行一次 getMore。实现处理该 getMore 的完整批次后才发送 Bookmark，因此切换点由一致快照和服务端 post-batch resume token 共同确定，而不是由轮询次数、延时或缓冲区状态猜测。调用方通过 UID 和对象 ResourceVersion 合并快照与该批次的重叠状态。MongoDB 不提供 Store 的整数全局 checkpoint，传入正数 WithResourceVersion 时返回 ResourceExpired。

## 实现与测试

直接实现通过 `store/storetest.Fixture` 提供隔离 Store 和预期 Capabilities。套件首先比较 Store 的能力声明，再验证基础契约和声明为支持的能力；未声明的能力不参与测试。

MongoDB、MySQL 和 PostgreSQL 套件使用顶层 `testkit` 启动或连接真实数据库；etcd 和 etcdcache 使用隔离前缀。实现特有的编码、索引、事务和 Watch 细节继续保留在各实现自己的测试中。

业务编号不属于 Store ID。需要仓库内 issue number 等单调序号时使用独立的 `sequence.Allocator`；对象 ID 仍使用 UUID。序号按调用方提供的稳定 key 隔离，保证单调且唯一，但允许空洞，也不保证与业务对象 Create 处于同一事务。
