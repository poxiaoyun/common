# Configuration command source design

This package is the adapter between runtime `config.DynamicConfig` and startup
`command.Source`. Neither owning package depends on the other.

One Source binds one DynamicConfig, namespace, and Configuration name.
The Configuration value uses the same document shape as command configuration
files: `global` selects Program global options and the selected command path
selects action options. Missing sections and a missing optional Configuration
contribute no SourceValue; an existing empty object remains an explicit empty
SourceValue. A malformed document is a startup error.

The adapter is not a default Source. A composition root may inject an existing
DynamicConfig, or use `FromOptions` with this package's `Options{Address,
Token}` for Configcenter control parameters. An empty Address uses
the Noop adapter; a non-empty address without a scheme uses HTTP. The Source
owns the `configcenter-address` and `configcenter-token` global control flags
and their `CONFIGCENTER_*` environment equivalents; flags override
environment values. `DefaultSources` composes it at the required precedence without changing
the command module's dependency direction.
