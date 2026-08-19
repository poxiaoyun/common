# HTTP adapter design

This package owns the HTTP implementation of `config.DynamicConfig`. Configcenter
Options and scheme selection belong to `config/commandsource`. This adapter maps namespace and name to canonical item routes,
write preconditions to `If-None-Match` or `If-Match`, Patch types to media
types, and SSE frames to `config.Event`.

The adapter does not own server authorization or public-read policy. A Watch
connection is state synchronization rather than an audit stream: reconnecting
creates a new Watch and receives a new Initial event.
