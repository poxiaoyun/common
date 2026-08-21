# asset/store

`New` returns an `asset.Service` backed by any `common/store.Store`.
`AddToSchema` must be called before constructing that Store.
Upload behavior is configured by `store.Options`.
