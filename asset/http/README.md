# asset/http

`NewServer` projects any `asset.Service` over HTTP. Mount `Group()` under the
authenticated API prefix and `PublicGroup()` wherever asset content should be
served.

`New(ctx, Options)` returns a remote implementation of `asset.Service`.
`Options.Address` is the API prefix containing both management and content
routes, and `Options.Token` supplies fixed bearer authentication. Services that
need runtime transport composition use `NewWithTransport(ctx, options, wrapper)`.
