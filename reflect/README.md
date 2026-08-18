# reflect

`reflect` adds reusable traversal and conversion operations to Go's standard
reflection package.

`ParseStruct` returns a semantic `Node` rooted at the supplied runtime value.
It applies the selected struct-tag precedence, removes ignored fields and nil
pointer fields, and flattens inline fields. Empty `omitempty` fields are removed
unless `Options.IgnoreOmitEmpty` keeps them in the runtime tree.
`Fields` contains the actual semantic children. Maps and collections also
expose their declared value shape directly through `Element`, so an empty
container still describes values that may be added later. Declared element
shapes do not apply `omitempty`, because an empty container has no element
value from which emptiness can be decided.

`MergePatch` applies an arbitrary patch directly to a pointed-to Go value.
Structs and string-keyed maps are semantic objects, maps merge recursively, nil
deletes map entries or clears nullable fields, and collections replace. Struct
patches use the supplied tags and the same runtime tree options. Strings remain
scalar values unless a non-string target explicitly implements
`encoding.TextUnmarshaler` or `json.Unmarshaler`; text decoding takes precedence.

`SetValue` assigns converted Go values, while `SetTextValue` parses text into a
target value. `FormatValue` performs the corresponding display conversion.
Types implementing the standard text interfaces use those interfaces,
`time.Duration` uses Go duration syntax, and remaining values use JSON.
