# Store adapter design

This package owns the persistence implementation of `config.DynamicConfig`.
It registers the exported `StoredConfiguration` Store object, selects the
namespace Store scope, maps version preconditions to Create, Update or Patch,
and converts Store metadata to the root contract's Name and Version.

The persistence type embeds `store.ObjectMeta`; `config.Configuration` does
not. It declares the stable resource name `configurations`, preserving existing
storage independently of its Go type name. Missing reads and deletes become an
empty Version 0 snapshot. Patch uses an empty object when no row exists and
atomically creates or changes the persisted value according to the requested
version.
