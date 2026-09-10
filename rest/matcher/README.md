# Matcher - 高性能路由匹配器

一个基于前缀树（Trie）的 HTTP 路由匹配器，支持路径变量、正则验证和贪婪匹配。

## 特性

- ✅ 共享路径前缀：基于前缀树定位候选
- ✅ 灵活匹配：支持路径变量、正则验证、贪婪匹配
- ✅ 智能优先级：自动按具体度排序路由
- ✅ 候选回退：按最终优先级逐个判断附加条件
- ✅ 并发安全：读操作无锁

## 语法

| 语法 | 说明 | 示例 |
|------|------|------|
| `/path` | 静态路径 | `/api/users` |
| `{name}` | 路径变量 | `/api/{id}` 匹配 `/api/123` |
| `{name:regex}` | 正则验证 | `/{id:[0-9]+}` 只匹配数字 |
| `{name}*` | 贪婪匹配 | `/{path}*` 匹配多个段 |
| `:action` | 自定义方法 | `/{resource}:batch` |

## 路由优先级

当多个路由都能匹配同一路径时，按以下优先级选择：

1. **根路径** `/` 优先级最高
2. **常量字符多** 的路径段优先
3. **有正则验证** 的变量优先
4. **变量少** 的路径段优先
5. **非贪婪** 优先于贪婪匹配

路径段从左到右比较，静态子树先于同层动态路径。例如 `/console/{rest}*` 会先于
`/{organization}/{type}/{repository}.git/{rest}*` 成为候选。附加条件由调用方的候选
回调判断；中间索引节点也可能到达回调，调用方可按节点值判断是否存在可用处理器。

### 示例 1：静态 vs 动态

```go
root.Register("/v1/nodes")
root.Register("/v1/{resource}")

root.Match("/v1/nodes")     → "/v1/nodes" (静态优先)
root.Match("/v1/pods")      → "/v1/{resource}" (resource=pods)
```

### 示例 2：多层优先级

```go
root.Register("/")                    // 优先级 1: 根路径
root.Register("/api/users")           // 优先级 2: 完全静态
root.Register("/api/{id:[0-9]+}")     // 优先级 3: 有正则验证
root.Register("/api/{id}")            // 优先级 4: 普通变量
root.Register("/api/{path}*")         // 优先级 5: 贪婪匹配

root.Match("/")              → "/"
root.Match("/api/users")     → "/api/users"
root.Match("/api/123")       → "/api/{id:[0-9]+}"
root.Match("/api/abc")       → "/api/{id}"
root.Match("/api/a/b/c")     → "/api/{path}*"
```

### 示例 3：复杂场景

```go
root.Register("/api/v1/users")              // 最具体
root.Register("/api/v1/{resource}")         // 部分静态
root.Register("/api/{version}/users")       // 部分静态
root.Register("/api/{version}/{resource}")  // 全变量

root.Match("/api/v1/users")      → "/api/v1/users"
root.Match("/api/v1/pods")       → "/api/v1/{resource}"
root.Match("/api/v2/users")      → "/api/{version}/users"
root.Match("/api/v2/pods")       → "/api/{version}/{resource}"
```

## 高级用法

### 复杂路径模式

```go
// Docker Registry API 风格
pattern := "/v2/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}"
root.Register(pattern)

// 匹配: /v2/library/nginx/manifests/latest
// 变量: repository=library/nginx, reference=latest
```

### 自定义匹配条件

```go
node, vars := root.Match("/api/users", func(val Handler, vars []MatchVar) bool {
    // 自定义过滤逻辑
    return val.Method == "GET"
})
```

候选回调接收该候选的完整路径变量，包括最后一段。返回 `false` 会继续按路径优先级
寻找候选；只有最终接受的候选及其变量会作为 `Match` 的结果返回。回调顺序遵循路径
段的具体度。需要在回调返回 `false` 后保留该候选变量的调用方，应复制变量切片。

## 性能

基准覆盖静态路径、动态变量、正则、贪婪匹配、候选优先级和并发读取：

```bash
# 运行所有匹配测试
go test -bench=BenchmarkMatch -benchmem

# 并发测试
go test -bench=BenchmarkMatchConcurrent -benchmem
```

## 设计原理

### 数据结构

```
前缀树（Trie）+ 优先级排序

根节点
├── /api (常量)
│   ├── /users (常量) [优先级高]
│   └── /{id} (变量) [优先级低]
└── /{service} (变量)
```

### 匹配流程

1. **分词**：将路径按 `/` 分割成 token
2. **树遍历**：从根节点开始，按路径段优先级尝试匹配子节点
3. **变量提取**：提取路径变量并验证非空及正则条件
4. **候选判断**：使用完整变量调用回调，返回首个接受的候选

### 优化技术

- **预分配**：减少切片扩容
- **Map 缓存**：O(1) 子节点查找
- **零拷贝**：使用字符串切片而非复制

## 限制

1. **普通变量不能为空**：`/{id}` 不匹配 `/`。贪婪变量允许空值，`/v2/{rest}*` 匹配 `/v2/`；不含分隔符的 `/v2` 仍需单独注册。
2. **字面后缀匹配段尾**：`/{repository}.git` 可匹配 `/model.git.backup.git`，变量为 `model.git.backup`；贪婪变量也可以带固定后缀。
3. **正则不能包含捕获组**：使用非捕获组 `(?:...)`

## 常见问题

### Q: 为什么 `/{service}` 不匹配 `/`？

A: 为了避免歧义。如果需要同时匹配，注册两个路由：

```go
root.Register("/")
root.Register("/{service}")
```

### Q: 如何匹配带点的路径？

A: 点号是普通字符，直接使用：

```go
root.Register("/files/{filename}.{ext}")  // 匹配 /files/doc.pdf
```
