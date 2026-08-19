# store

`config/store` implements `config.DynamicConfig` with `common/store`.

```go
schema := store.NewSchema()
if err := configstore.AddToSchema(schema); err != nil {
    return err
}
client := configstore.New(storage)
```

The adapter stores `config.Configuration` in the `namespaces/{namespace}`
scope. It derives the `configurations` resource name from the type, supports
atomic Store patches and translates Store initial events into the
`config.DynamicConfig` Watch contract.
