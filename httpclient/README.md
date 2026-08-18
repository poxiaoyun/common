# HTTP client

`httpclient` provides request construction and transport helpers shared by Go
clients.

Use `ListAll` when a caller needs every item from a resource list. The supplied
list function may return page-style, continuation-style, or one-shot responses;
`ListAll` follows the response metadata until the server reports completion.
Filters, selectors, search, and sort options are preserved across requests.

If any request fails, `ListAll` returns the error and no partial page. The
returned page contains all items and a `Total` equal to their count; traversal
metadata is cleared.
