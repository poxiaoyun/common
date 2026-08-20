# noop

`config/noop` implements the disabled state of `config.DynamicConfig`. Get
returns a Version 0 empty Configuration, ListKeys returns an empty list, Watch
sends the same empty snapshot and then waits for cancellation, and mutation
methods return Unsupported.

```go
client := noop.New()
```

`config/commandsource` selects this adapter when its bootstrap Address is empty.
