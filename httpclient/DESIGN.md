# HTTP client design

## Pagination seam

`ListAll` owns client-side traversal of paginated list responses. Callers supply
one list operation and ordinary `meta.ListOptions`; they do not reproduce page
or continuation-token state machines.

The response metadata selects the next request. A non-empty continuation token
takes precedence, otherwise a positive page number advances while `Total`
shows more results. A response with neither ends traversal. Empty continuation
batches are valid and must still advance.

`ListAll` returns no partial result after an error. Its aggregate collection
resource version is retained only when every response reports the same positive
version, because a mixed-version result is not one collection snapshot.
