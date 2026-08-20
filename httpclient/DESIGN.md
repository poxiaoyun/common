# HTTP client design

## Pagination seam

`ListOptionsToQuery` is a transport projection. It serializes every non-zero
list field without selecting, validating, or normalizing pagination behavior;
the service executing the list owns those semantics.

`ListAll` owns client-side traversal of paginated list responses. Callers supply
one list operation and ordinary `meta.ListOptions`; they do not reproduce page
or continuation-token state machines. `ListAll` trusts both its options and
returned pagination metadata and does not validate them.

The response metadata selects the next request. A positive `Limit` identifies a
continuation response: a non-empty `Continue` advances, while an empty token
ends traversal. Otherwise a positive `Page` advances while `Total` shows more
results. A response with neither is a one-shot result and ends traversal. Empty
continuation batches are valid and must still advance when they carry a token.

`ListAll` returns no partial result after an error. Its aggregate collection
resource version is retained only when every response reports the same positive
version, because a mixed-version result is not one collection snapshot.
