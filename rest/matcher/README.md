# 路径匹配器

`rest/matcher` 编译路径模式，通过共享前缀树枚举候选路由。匹配范围由模式完整表达，
不清理请求路径，也不执行重定向。

| 模式 | 含义 |
| --- | --- |
| `/v2` | 仅匹配 `/v2` |
| `/v2/` | 匹配 `/v2/` 及其所有子路径 |
| `/v2/{$}` | 仅匹配 `/v2/` |
| `/{$}` | 仅匹配根路径 `/` |
| `/` | 匹配所有路径 |
| `/files/{name}` | 捕获一个非空路径段 |
| `/files/{rest...}` | 捕获剩余路径，可跨多个段，也可为空 |

`/v2/` 和 `/v2/{rest...}` 都不匹配无尾斜杠的 `/v2`。需要直接处理两种地址时，
分别注册 `/v2` 和 `/v2/`。尾部 `/` 是匿名多段匹配，不生成路径变量。
变量名必须为唯一的 Go 标识符；`{$}` 只能出现在最后一个完整路径段。

common 在基础语法上支持这些扩展：

- 复合段：`/{repository}.git`、`/{name}:exec`、`/v{version}`。
- 正则约束：`/{id:[0-9]+}`，约束整个捕获值；正则使用 Go `regexp` 语法。
- 中间多段捕获：`/{scopes...}/roles/{role}`。
- 多段正则约束：`/{scopes...:[a-z/]+}/roles`。
- 字面前缀通配：`/assets/prefix-*.css`。

中间多段捕获以随后出现的字面分隔符结束；末尾字面后缀匹配剩余部分的结尾。
例如 `/{repository}.git` 匹配 `/model.git.backup.git`，捕获 `model.git.backup`。
这些扩展不属于标准 `net/http.ServeMux` 的语法。

## 使用与捕获

```go
root := &matcher.Node[string]{}
_, node, err := root.Register("/files/{rest...}")
if err != nil {
    return err
}
node.Value = "files"

matched, vars := root.Match("/files/models/a.bin", nil)
// matched.Value == "files"；vars 包含 rest=models/a.bin。
```

`Match` 接收 URL 的 escaped path，HTTP 调用方传入 `r.URL.EscapedPath()`。
模式与请求按路径段比较解码后的字符，编码斜杠 `%2F` 不成为路径分隔符。
捕获值仅解码一次：`a%2Fb` 得到 `a/b`，`%252F` 得到 `%2F`。
匹配不修改调用方的原始 URL，代理可保留其编码、分隔符和查询串。

可提供候选回调决定是否接受路由：

```go
matched, vars := root.Match(request.URL.EscapedPath(), func(value string, captures []matcher.MatchVar) bool {
    return accepts(value, request)
})
```

回调只接收已注册路径及其完整捕获值。返回 false 会继续寻找候选。
若拒绝候选后仍需保留其变量，应复制切片。注册在开始处理请求前完成，处理请求时不修改树。

## 候选顺序与文档

路径段从左到右比较：常量更多、有正则约束、非贪婪和变量更少的段依次优先。
精确的根或尾斜杠路径先于同位置的通配匹配。静态子树先于同层动态路径，
例如 `/console/` 先于 `/{organization}/{type}/{repository}.git/` 成为候选。
方法、媒体条件和显式 Route Priority 由 `rest/api` 统一决定。

`CompilePattern` 返回编译后的段，`PathTemplate` 将其投影为文档路径：保留变量名，
去掉正则、多段标记和 `{$}`。它不会修改原始模式。匿名子树只提供挂载路径，
完整的通配覆盖范围不由 OpenAPI 路径模板表达。
