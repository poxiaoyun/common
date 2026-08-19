# noop

`config/noop` implements the disabled state of `config.DynamicConfig`. Get
always reports a missing Configuration, Watch reports an initial missing state
and then waits for cancellation, and mutation methods return Unsupported.

```go
client := noop.New()
```

`config/commandsource` selects this adapter when its bootstrap Address is empty.
