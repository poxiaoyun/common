# store

`config/store` implements `config.DynamicConfig` with `common/store`.

```go
schema := store.NewSchema()
if err := configstore.AddToSchema(schema); err != nil {
    return err
}
client := configstore.New(storage)
```

The adapter stores an exported `StoredConfiguration` persistence object in the
`namespaces/{namespace}` scope so callers can use the same object with
`common/store` directly. Its resource is `configurations`; Store metadata is
converted to the public Name and Version fields. Missing reads and deletes
become Version 0 empty snapshots, and Store initial events become the first
snapshot required by `config.DynamicConfig`.
