# Store adapter design

The adapter owns the Store persistence representation named `Asset`. Its type
name naturally maps to resource `assets`; Store scopes remain `kind` and
`owner`. Content is base64 encoded in the same record so the implementation
works with every `common/store.Store`. Link-backed Assets persist the direct
Link in that record instead of downloading it.
