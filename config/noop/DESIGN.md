# Noop adapter design

This package represents the explicit disabled configuration-center state. It
does not persist values or start network activity. Reads return the requested
name with Version 0 and an empty object, Watch sends that snapshot once, and
writes fail as unsupported rather than pretending to persist data. ListKeys
returns an empty list.
