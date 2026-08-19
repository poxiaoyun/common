# Store adapter design

This package owns the persistence implementation of `config.DynamicConfig`.
It registers `config.Configuration`, selects the namespace Store scope, maps
write preconditions to Create, Update or Patch, and hides Store bookmarks and
watch checkpoints behind the contract's Initial event.

`config.Configuration` is used directly as the Store object. The adapter does
not introduce a persistence-only duplicate and does not override Store resource
name inference. JSON Merge Patch is wrapped at the `value` field; JSON Patch
paths are rooted below `/value`, so metadata cannot be patched.
