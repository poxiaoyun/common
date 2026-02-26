# Matcher - 高性能路由匹配器

一个基于前缀树（Trie）的 HTTP 路由匹配器，支持路径变量、正则验证和贪婪匹配。

## 特性

- ✅ 高性能：基于前缀树，O(log n) 查找复杂度
- ✅ 灵活匹配：支持路径变量、正则验证、贪婪匹配
- ✅ 智能优先级：自动按具体度排序路由
- ✅ 零分配：静态路由匹配零内存分配
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
2. **常量字符多** 的路由优先
3. **有正则验证** 的变量优先
4. **变量少** 的路由优先
5. **非贪婪** 优先于贪婪匹配

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

## 性能

基准测试结果（Apple M1）：

| 场景 | 时间/op | 内存/op | 分配次数 |
|------|---------|---------|---------|
| 根路径匹配 | 31 ns | 16 B | 1 |
| 静态简单路由 | 51 ns | 32 B | 1 |
| 优先级测试 | 52 ns | 32 B | 1 |
| 静态复杂路由 | 87 ns | 64 B | 1 |
| 单变量路由 | 95 ns | 96 B | 3 |
| 多路由匹配 | 126 ns | 112 B | 3 |
| 正则验证 | 161 ns | 96 B | 3 |
| 贪婪匹配 | 205 ns | 248 B | 5 |
| 多变量路由 | 238 ns | 384 B | 7 |
| 复杂模式 | 343 ns | 456 B | 8 |
| 并发匹配 | 56 ns | 132 B | 3 |

运行基准测试：

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
2. **树遍历**：从根节点开始，按优先级尝试匹配子节点
3. **变量提取**：匹配成功时提取路径变量
4. **正则验证**：如果有正则表达式，验证变量值

### 优化技术

- **预分配**：减少切片扩容
- **Map 缓存**：O(1) 子节点查找
- **零拷贝**：使用字符串切片而非复制

## 限制

1. **变量不能为空**：`/{id}` 不匹配 `/`（使用 `/` 和 `/{id}` 两个路由）
2. **贪婪匹配必须在末尾**：`/{path}*/suffix` 不支持
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