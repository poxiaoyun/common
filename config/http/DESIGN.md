# HTTP adapter design

This package owns the HTTP implementation of `config.DynamicConfig`. Configcenter
Options and scheme selection belong to `config/commandsource`. This adapter maps namespace and name to canonical item routes,
version preconditions to `If-None-Match` or `If-Match`, Patch types to media
types, and streamed configuration snapshots to `config.Event`. Transport event
kinds do not enter the root contract.

`New` owns fixed Bearer authentication. `NewWithTransport` preserves the
adapter's configured base transport while allowing a caller to compose a
rotating service identity.

The adapter does not own server authorization or public-read policy. A Watch
connection is state synchronization rather than an audit stream: reconnecting
creates a new Watch and receives the current snapshot first, including an empty
Version 0 snapshot when the configuration is missing.
