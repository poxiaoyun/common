# Reflection design

The package owns semantic reflection mechanics shared by callers: pointer
indirection, struct-tag resolution, ignored and inline fields, optional
`omitempty` filtering, container traversal, and scalar classification.

Tag precedence selects the first present tag as the complete authoritative
contract. Lower-priority tags do not contribute names or options once a
higher-priority tag exists.

`Node` is the authoritative result. It directly contains actual children in
`Fields` and the declared element shape of dynamic containers in `Element`.
There is no separate tree wrapper, child index, or accessor seam. Callers keep
only their domain metadata and must not reconstruct the reflection hierarchy.

Runtime parsing uses the supplied value to determine the visible shape. It
honors the selected tag's `omitempty` option unless `Options.IgnoreOmitEmpty`
disables that serialization-specific filtering. Nil pointers remain absent in
both modes. Type-only element parsing intentionally retains `omitempty` fields
because an empty map or slice has no runtime element from which emptiness can
be decided.

`SetValue`, `SetTextValue`, and `FormatValue` own value conversion. Text input
uses `encoding.TextUnmarshaler` before `json.Unmarshaler` because command-line
and environment values are text rather than JSON documents. Text output uses
the corresponding `encoding.TextMarshaler`; `time.Duration` is the standard
exception and uses `String`/`time.ParseDuration`. Other values use their JSON
representation. Parsing and formatting rules therefore remain local to one
module instead of being repeated by configuration sources.

`MergePatch` is the mutation boundary. It accepts a pointer target and any
semantic patch value. Structs and string-keyed maps merge as objects, while
collections replace and scalars use `SetValue`. Callers do not need to
construct or traverse a `Node` to apply a patch.
