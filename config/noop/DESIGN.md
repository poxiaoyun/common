# Noop adapter design

This package represents the explicit disabled configuration-center state. It
does not persist values or start network activity. Reads follow the contract's
missing-value semantics, Watch remains at an initial missing state, and writes
fail as unsupported rather than pretending to persist data.
