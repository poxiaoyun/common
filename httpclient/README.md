# HTTP client

`httpclient` provides request construction and transport helpers shared by Go
clients.

`ListOptionsToQuery` 只负责把非零 `meta.ListOptions` 字段转换成平铺 query；它不判断
分页组合、不选择分页行为，也不规范化字段值。

Use `ListAll` when a caller needs every item from a resource list. The supplied
list function may return page-style, continuation-style, or one-shot responses;
`ListAll` follows the response metadata until the server reports completion.
Filters, selectors, search, and sort options are preserved across requests.
`ListAll` consumes its options and responses without validation.

If any request fails, `ListAll` returns the error and no partial page. The
returned page contains all items and a `Total` equal to their count; traversal
metadata is cleared.
