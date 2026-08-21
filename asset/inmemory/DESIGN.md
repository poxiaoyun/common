# In-memory adapter design

The adapter owns an isolated content map keyed by target kind, target name, and
asset name. It implements the complete `asset.Service` behavior without
exposing its storage representation. Content-backed Assets copy the readable
bytes; Link-backed Assets retain the direct Link.
