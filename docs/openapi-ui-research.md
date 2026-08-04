# OpenAPI 文档 UI 选型调研

> 调研日期：2026-08-04。仅采用项目官方仓库、官方文档、许可证和官方发布物。

## 结论

推荐用 **Scalar API Reference** 渲染本仓库原生生成的 OpenAPI 3.1 文档。它是当前候选中兼顾现代交互、活跃维护、OpenAPI 3.1、Try It、OAuth2/PKCE、搜索和自托管能力最完整的方案。

建议顺序：

1. **Scalar**：本仓库默认 UI 的首选替代品。
2. **Stoplight Elements**：需要 React/Web Component 组件化嵌入时的次选。
3. **Redoc CE**：只读 API reference 很好，但开源版没有真正的 Try It。
4. **Swagger UI**：仍活跃且成熟，只是体验较传统；本仓库不再保留兼容回退。
5. **RapiDoc**：功能齐全但默认分支自 2024-11 后无代码更新，不建议新采用。
6. **Zudoku**：适合独立开发者门户，不适合当前 `embed.FS + 单 HTML` 的低成本替换。

最终实现选择一次性完成协议和 UI 迁移：[`openapi/plugin.go`](../openapi/plugin.go) 直接生成 OpenAPI 3.1，不再生成、转换或暴露 Swagger 2.0 文档。

## 本仓库现状

- [`openapi/plugin.go`](../openapi/plugin.go) 使用 `kin-openapi/openapi3` 直接生成并校验 OpenAPI 3.1.1。
- [`openapi/ui.go`](../openapi/ui.go) 只暴露 Scalar 页面和同路径的 `openapi.json`，没有 provider 分支。
- Scalar 1.64.0 standalone 资产固定版本后放入 `embed.FS`，记录了包完整性、文件 SHA-256 和 MIT 许可证。
- Swagger UI、Redoc、Stoplight 页面和旧 Swagger UI 静态资产均已移除；运行时不依赖公网 CDN。

## 候选对比

| 项目 | 开源与活跃度快照 | OAS / Swagger | 交互、认证与搜索 | 嵌入与定制 | 判断 |
| --- | --- | --- | --- | --- | --- |
| **Scalar API Reference** | [MIT](https://github.com/scalar/scalar/blob/main/LICENSE)；约 15.8k stars；[1.64.0 / 2026-07-31](https://github.com/scalar/scalar/releases/tag/release-2026-07-31-dc1e98f)；[2026-08-03 仍提交](https://github.com/scalar/scalar/commit/7d9b083607b83aaa441fd0a9a5da21dc21a8ffe5) | 官方明确支持 [Swagger 2.0、OAS 3.0、3.1](https://github.com/scalar/scalar/blob/main/documentation/openapi.md) | 内置 API client、代码样例、搜索；API key/basic/bearer、OAuth2 各 flow 和 PKCE，见[配置](https://github.com/scalar/scalar/blob/main/documentation/configuration.md) | HTML/JS mount、npm；11 个主题、现代/经典布局、custom CSS；可完全自托管 | **首选**；升级成本低且产品体验最现代 |
| **Stoplight Elements** | [Apache-2.0](https://github.com/stoplightio/elements/blob/main/LICENSE.md)；约 2.4k stars；npm [9.0.24 / 2026-07-14](https://www.npmjs.com/package/@stoplight/elements) | 官方矩阵列出 [Swagger 2.0、OAS 3.0、3.1](https://github.com/stoplightio/elements#readme) | Try It、自动样例和多种 auth；单 API 搜索偏导航过滤，完整站点搜索属于 portal 组件 | React 与 Web Component、sidebar/stacked 布局、CSS 定制 | 组件化很好；本仓库已有页面，但整体体验和安全默认值不如 Scalar 清晰 |
| **Redoc CE** | [MIT](https://github.com/Redocly/redoc/blob/main/LICENSE)；约 25.8k stars；[2.5.3 / 2026-05-29](https://github.com/Redocly/redoc/releases/tag/v2.5.3)；[2026-07-23 仍提交](https://github.com/Redocly/redoc/commit/ed30fc53d2f9a8ba24a5fb5a0f50150091767cc9) | [Swagger 2.0、OAS 3.0、3.1](https://github.com/Redocly/redoc#readme) | 搜索和安全定义展示良好；官方明确把 [Try It 和自动代码样例列为商业版能力](https://github.com/Redocly/redoc#redoc-vs-redocly-api-reference) | standalone HTML、npm/React、CLI、Docker；主题能力强 | 只读文档优选，不适合作为 Swagger UI 的交互替代 |
| **Swagger UI** | [Apache-2.0](https://github.com/swagger-api/swagger-ui/blob/master/LICENSE)；约 28.9k stars；[5.32.12 / 2026-08-03](https://github.com/swagger-api/swagger-ui/releases/tag/v5.32.12) | 官方[兼容表](https://github.com/swagger-api/swagger-ui#compatibility)覆盖 Swagger 2.0 和 OAS 3.0/3.1 | Try It、API key、OAuth2/OIDC、filter 搜索、拦截器和插件都成熟，见[配置](https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/)与[OAuth2](https://swagger.io/docs/open-source-tools/swagger-ui/usage/oauth2/) | dist/npm/React，自托管成熟；主题主要依靠 CSS/插件 | 不是失去维护，而是 UI/信息架构较传统；本仓库不采用 |
| **RapiDoc** | [MIT](https://github.com/rapi-doc/RapiDoc/blob/master/LICENSE.txt)；约 1.9k stars；[9.3.8 / 2024-10-11](https://github.com/rapi-doc/RapiDoc/releases/tag/v9.3.8)；默认分支最后代码提交为 [2024-11-06](https://github.com/rapi-doc/RapiDoc/commit/7f53d25959e5a4e1beb4b610aaef445b896838f2) | README 声明 Swagger 2.0/OAS 3.x，仓库有 [OAS 3.1 示例](https://github.com/rapi-doc/RapiDoc/blob/master/docs/examples/open-api-3-1.html) | Try、basic/bearer/API key/OAuth、普通和高级搜索，见[属性文档](https://github.com/rapi-doc/RapiDoc/blob/master/docs/api.html) | 单一 Web Component；发布物约 865 KiB minified / 218 KiB gzip；light/dark 和大量属性 | 轻量，但维护信号明显弱于其他候选 |
| **Zudoku** | [MIT](https://github.com/zuplo/zudoku/blob/main/LICENSE.md)；约 570 stars；[0.82.5 / 2026-07-28](https://github.com/zuplo/zudoku/releases/tag/v0.82.5) | 官方支持从一个或多个 OpenAPI 文档生成 reference，但没有同等清晰的版本兼容矩阵 | Playground、API key/bearer；OAuth2/OIDC 通过 portal auth/provider 配置；Pagefind/Algolia/Inkeep 搜索 | npm 应用框架、MDX、插件、主题、静态构建和自托管 | 完整 portal 候选；引入 Node 构建和路由体系，迁移成本远高于本次目标 |

## Bundle、依赖与 CSP

发布包的 unpacked size 只能描述依赖形态，不等同浏览器首屏：Scalar 约 11.1 MiB、Swagger UI dist 约 11.7 MiB、Redoc 约 7.8 MiB、RapiDoc 约 3.6 MiB、Stoplight Elements 约 2.8 MiB、Zudoku 约 3.8 MiB；具体应以 vendor 后的 gzip/brotli 和浏览器 trace 为准。来源为各官方 npm 发布页：[@scalar/api-reference](https://www.npmjs.com/package/@scalar/api-reference)、[@stoplight/elements](https://www.npmjs.com/package/@stoplight/elements)、[redoc](https://www.npmjs.com/package/redoc)、[rapidoc](https://www.npmjs.com/package/rapidoc)、[swagger-ui-dist](https://www.npmjs.com/package/swagger-ui-dist)、[zudoku](https://www.npmjs.com/package/zudoku)。

- Scalar 的浏览器 standalone 是较大的单包，也提供 ESM/chunks；本仓库应选固定版本后实测，而不是引用 `latest`。
- RapiDoc 是最简单的单 JS Web Component；Stoplight 是 React 组件树/Web Component；Redoc 和 Swagger UI 均有成熟 standalone。
- Scalar 已合入 [CSP nonce 支持](https://github.com/scalar/scalar/pull/9422)，但运行时 inline style 仍需要验证 `style-src` 策略。
- Swagger UI 的严格 CSP 仍有 [inline style](https://github.com/swagger-api/swagger-ui/issues/3370) 等长期问题；其他候选未找到正式的严格 CSP 保证，应通过 POC 验证。
- Stoplight 包含可关闭的 Scarf 安装统计；官方说明可用 `SCARF_ANALYTICS=false` 禁用，见其 [README](https://github.com/stoplightio/elements#anonymized-analytics)。

## 推荐的 Scalar 安全配置

固定并内嵌 `@scalar/api-reference@1.64.0` 的浏览器资产，不使用运行时 CDN，也不要设置 Scalar 的公网 proxy：

```js
Scalar.createApiReference("#app", {
  url: "openapi.json",
  withDefaultFonts: false,
  telemetry: false,
  agent: { disabled: true },
  persistAuth: false,
});
```

原因：默认字体来自 Scalar CDN；开源自托管版没有加载 analytics plugin 时不会跟踪，但显式关闭更稳妥；Agent 在 localhost 默认可出现，并会在首次提问时上传 OpenAPI 文档，官方配置说明见 [Agent 与 telemetry](https://github.com/scalar/scalar/blob/main/documentation/configuration.md)。初始化脚本应作为本地静态文件或带 nonce，不在 HTML 中保留无 nonce inline script。

## 最终迁移方案

1. 文档模型直接使用 `openapi3.T`，schema 复用 `components.schemas` 和 `#/components/schemas/...` 引用。
2. `api.Route` 直接投影为 OAS3 operation、parameters、requestBody/content 和 responses/content，不经过中间 Swagger 2 模型。
3. body、form、multipart/file、数组参数、递归 schema、动态 interface overlay 和多 content type 由 OAS3 测试覆盖。
4. 固定并内嵌 Scalar standalone，关闭公网字体、Agent 和 telemetry，不设置公网 proxy。
5. 删除 provider 选择、Swagger UI fallback、Redoc/Stoplight CDN 页面以及所有 Swagger 2 生成和转换代码。
6. 后续浏览器验收继续覆盖 OAuth2/PKCE、CORS、错误响应、文件下载、深链、移动端和严格 CSP。

## 官方来源索引

- [Scalar 仓库与 quickstart](https://github.com/scalar/scalar#readme)、[配置](https://github.com/scalar/scalar/blob/main/documentation/configuration.md)、[主题](https://github.com/scalar/scalar/blob/main/documentation/themes.md)
- [Stoplight Elements 仓库与兼容矩阵](https://github.com/stoplightio/elements#readme)
- [Redoc CE 仓库、部署方式和商业版边界](https://github.com/Redocly/redoc#readme)
- [Swagger UI 官方仓库](https://github.com/swagger-api/swagger-ui)、[安装](https://swagger.io/docs/open-source-tools/swagger-ui/usage/installation/)
- [RapiDoc 官方仓库](https://github.com/rapi-doc/RapiDoc)、[属性/API](https://github.com/rapi-doc/RapiDoc/blob/master/docs/api.html)
- [Zudoku 官方仓库](https://github.com/zuplo/zudoku)、[API reference 配置](https://github.com/zuplo/zudoku/blob/main/docs/pages/docs/configuration/api-reference.md)、[部署](https://github.com/zuplo/zudoku/blob/main/docs/pages/docs/deployment.md)
