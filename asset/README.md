# Asset

`asset` 用于管理对象拥有的具名二进制内容，例如头像、Logo、附件和安装包。
对象由 `Target{Kind, Name}` 标识，同一对象可以拥有多个不同名称的 Asset。

`asset.Service` 是统一接口；应用在组合根选择 Store、S3、HTTP 或内存
适配器，业务代码不依赖具体存储。

## 基本使用

固定名称表示“这个对象的当前附件”。再次以相同名称调用 `Put` 会替换内容，
并递增 `Asset.Version`：

```go
target := asset.Target{Kind: "user", Name: userID}

current, err := assets.Put(ctx, target, asset.Blob{
    Content:     avatar,
    ContentType: "image/png",
}, asset.PutOptions{Name: "avatar"})
if err != nil {
    return err
}
```

名称留空表示创建一个新附件，调用方使用返回的 `Asset.Name` 继续访问它。
需要保留多个历史文件时，应为每次上传创建新名称，不要覆盖同一个名称。

`Get` 和 `List` 只读取描述信息；`Resolve` 才读取内容：

```go
resolved, err := assets.Resolve(ctx, target, current.Name, asset.ResolveOptions{
    Version: current.Version,
})
if err != nil {
    return err
}

if resolved.Link != nil {
    // 在 ExpiresAt 前访问 resolved.Link.URL。
} else {
    defer resolved.Content.Close()
    // 读取 resolved.Content。
}
```

Version 只标识当前内容的代际，不是历史版本。替换同名 Asset 后，旧 Version
不再解析到新内容，也不能用来读取替换前的内容。

## HTTP 缓存怎么选

`asset/http.Server` 在宿主挂载的前缀下提供内容地址：

```text
/assets/{kind}/{target}/{asset}
```

调用方只需要在以下两种模式中选择：

| 场景 | 地址 | 响应策略 | 调用方行为 |
| --- | --- | --- | --- |
| 缓存有效期内完全离线使用 | `...?v={Asset.Version}` | `Cache-Control: public, max-age=2592000` | 原样保存完整地址。缓存可在 30 天内直接使用；内容更新后切换到包含新 Version 的地址。 |
| 始终读取当前内容，并使用 304 节省传输 | 不带 `v` 的稳定地址 | `Cache-Control: no-cache`，并携带可用的 ETag 和 Last-Modified | 每次使用前访问服务端重新验证；未变化时服务端返回 304，客户端继续使用本地内容。 |

`no-cache` 表示可以保存响应，但使用前必须重新验证，不表示
`no-store`。

头像、Logo 等可变资源通常使用版本地址。资源更新成功后，资源所有者发布
包含新 Version 的完整地址，消费者用新地址替换旧地址。不要删除 `v`
参数，也不要另外添加时间戳参数。

服务不会主动清理已经下发的缓存。旧版本地址如果已经被缓存，可以继续返回
旧内容直到缓存过期；如果请求到达服务端，则因为 Version 已经过期而返回
Not Found。

上述缓存 Header 只适用于 Content 响应。使用 S3 预签名 Link 时，Asset
内容路由返回临时 307，最终内容由 S3 交付。需要由 Asset 路由提供离线缓存
时，应配置 `Proxy=true`，或在通用内容地址上使用 `link=false` 取得
Content。预签名 URL 可能过期，调用方应保存 Asset 地址，而不是保存重定向
目标。

版本响应使用公共缓存，因此只适合允许共享缓存保存的内容。

宿主提供头像等领域专用地址时，通过 `asset/http.ContentResponse` 输出
`Resolve` 结果：精确校验了正数 Version 的地址传入
`versioned=true`；表示当前内容的稳定地址传入 `false`。

## 选择适配器

| 场景 | 选择 |
| --- | --- |
| 内容与现有 `common/store.Store` 一起保存 | `asset/store`；构造 Store 前调用 `AddToSchema` |
| 内容保存在兼容 S3 的对象存储 | `asset/s3` |
| 通过另一个进程提供的 Asset 服务访问内容 | `asset/http` |
| 单元测试或允许退出后丢失的临时数据 | `asset/inmemory` |

S3 的 `Proxy=false` 允许 `Resolve` 返回预签名 URL，适合客户端可以直接
访问 S3 的部署；`Proxy=true` 让内容始终经过应用，适合 S3 只能由服务端
访问的部署。

HTTP Server 可以包装任意本地适配器：

```go
assets := assetstore.New(storage, assetstore.Options{})
server := assethttp.NewServer(assets)
```

远端 Go 调用方使用 `asset/http.Client`，仍然通过同一个
`asset.Service` 接口操作。

## 上传和标识约束

`Blob` 必须且只能设置 `Content` 或 `Link` 之一，并且必须提供
`ContentType`。上传策略由具体适配器配置；`AllowedMediaTypes` 支持
`image/*` 这样的媒体类型范围。

Asset 名称和 Target Kind 长度为 1–64，只能使用字母、数字、句点、
下划线和连字符，且首尾必须是字母或数字。Target Name 可以由一段或两段
最长 128 字符的同类标识组成，两段使用一个冒号分隔，例如
`cloud:database`。
