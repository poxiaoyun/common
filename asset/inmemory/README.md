# asset/inmemory

`New` returns an in-process implementation of `asset.Service`. It is intended
for tests and local processes; state is lost when the process exits.
Upload behavior is configured by `inmemory.Options`.
