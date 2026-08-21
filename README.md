# common

`common` 提供多个 Go 服务共同使用的基础类型、应用组件和测试设施。

## 常用包索引

这里列出职责明确、适合作为调用入口的包，而不是仓库中所有包的完整清单。

| 包 | 用途 |
| --- | --- |
| [`store`](./store) | 面向结构化资源的通用数据存储接口，支持作用域、查询、乐观并发控制和变更监听；包含内存、SQL、MongoDB、etcd 和 REST 实现 |
| [`rest/api`](./rest/api) | REST API 路由、请求处理以及认证、授权和审计扩展 |
| [`errors`](./errors) | 统一的状态错误、HTTP 状态码和错误原因判断 |
| [`log`](./log) | 基于上下文的结构化日志入口 |
| [`meta`](./meta) | 统一的对象元数据、分页结果和列表查询选项，调用方应优先复用这些公共类型 |
| [`controller`](./controller) | 基于队列和数据源的协调循环、并发控制及控制器管理 |
| [`httpclient`](./httpclient) | HTTP 客户端配置、认证 Transport 和请求辅助能力 |
| [`config`](./config) | 命令行参数、环境变量和动态配置加载 |
| [`asset`](./asset) | 命名二进制附件、稳定 URL、HTTP Client 以及 Store、S3、内存适配器 |
| [`cache`](./cache) | 过期值缓存、原子过期计数与固定窗口限流；包含进程内值缓存适配器 |
| [`validation`](./validation) | 结构体规则校验、常用字符串校验和字段路径转换 |
| [`i18n`](./i18n) | 基于 context 的国际化，支持翻译文件、模板、复数、回退和 HTTP 语言检测 |
| [`version`](./version) | 提供构建版本、Git 提交、Go 版本和运行平台信息 |
| [`rand`](./rand) | 生成密码学安全的随机字符串、数字和密码 |
| [`retry`](./retry) | 提供受 context 控制的固定间隔和退避重试 |
| [`pprof`](./pprof) | 提供独立的 expvar 与 pprof 调试处理器和服务 |
| [`task`](./task) | 异步任务的提交、执行和管理；包含内存与 MongoDB 实现 |
| [`eventbus`](./eventbus) | 异步事件发布与订阅；包含内存与 MongoDB 实现 |
| [`lease`](./lease) | 基于 Store 的分布式锁和 Leader Election |
| [`authn`](./authn) | 登录、会话和身份认证相关接口及 API |
| [`oidc`](./oidc) | OpenID Connect/OAuth 2.0 客户端操作和令牌校验 |
| [`jsonschema`](./jsonschema) | JSON Schema 2020-12 的表示、处理和校验 |
| [`testkit`](./testkit) | 为集成测试准备容器和数据库等外部依赖 |

## 文档约定

根 README 用于发现和选择包。具体包的使用方式、保证和失败语义以该目录下的
`README.md` 和 Go 文档为准；面向维护者的设计约束与决策记录在 `DESIGN.md`。
