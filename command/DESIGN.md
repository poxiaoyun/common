# Command design

`command` owns complete executable startup. `Program` and `Command` are
declarative specifications. `Exec` interprets one Program for one Execution;
the specifications contain no compiled or per-execution state.

## Model

A `Command` is the public definition-tree node. Callers directly set its name,
summary, children, positional-argument declarations, options, and run function
in a struct literal. A node with children participates in routing; a node with a
run function is executable; a node may have both. Before the current action is
selected, an exact child name routes to that child. A positional argument, an
action flag, or `--` selects the current action, after which child names are
ordinary positional values. A node with neither a run function nor children
renders help. There is no private definition tree, command builder, or
configured-action adapter.

Positional arguments are ordered `Argument` declarations on an executable
Command. Each declaration supplies the semantic name shown by help and may be
optional or variadic; a variadic argument is last, and an optional argument is
not followed by a required one. An empty declaration rejects positional
arguments. Routing, invocation validation, help, and future completion all
consume this one specification. `Invocation.Arguments` remains the ordered
runtime values rather than introducing a second bound-argument representation.

An Argument may declare a completion function. Its input contains the selected
command path, raw words before the cursor, already parsed positional arguments,
and the current prefix. This is passive specification data until a completion
interpreter is added; it does not add completion behavior to `Exec` or make
completion a Plugin lifecycle hook.

A `Program` embeds its root `Command` and declares Plugins, Sources, and
unknown-property policy. Nil Plugins and Sources select their defaults; an
explicit empty slice disables them. `Exec` derives all routing, flags, and
configuration schema state for that execution.

An `Execution` supplies one run's arguments, environment, file reader, and
streams. After routing selects an action, configured actions create fresh
defaults and apply the Program's `Source` values from low to high priority.
The resulting options are stored privately on `Invocation` and exposed to the
action through `Options[T](Invocation)`. The command tree necessarily erases
their concrete types because different actions may use different option
structs; the generic accessor keeps that assertion inside the command module.
Ordinary actions do not run configuration sources.

`Invocation` contains an explicit context, remaining positional arguments,
resolved options, and streams. It is not itself a `context.Context`.

## Plugin seam

Plugins own command behavior that does not produce typed configuration. `Exec`
compiles Plugin flags and the selected action's Source flags, scans arguments
once to route and assign every flag to its owner, then runs Plugins, Sources,
and the Action in that order.

The base `Plugin` only supplies a diagnostic name. `CommandPlugin` contributes
root commands before validation and routing. `FlagPlugin` contributes global
flags and handles its values after parsing; `CommandErrorPlugin` transforms
structured parsing and routing errors. These are capability interfaces rather
than one lifecycle interface with unused hooks.

Logging, Version, Help, and command suggestions are default Plugins. Logging
places the current logger in the execution context and owns `-v`/`--v`; Version
owns `--version` and contributes the `version` command;
Help owns `-h`/`--help` and consumes the execution's read-only `CommandRuntime`;
command suggestions consume `UnknownCommandError`. Nil Plugins select these
defaults; an explicit empty slice disables them.

## Source seam

Every configuration input implements the same `Source` interface and returns
ordered `SourceValue` values. Files, environment variables, command-line
configuration, and external adapters have no privileged execution path and
cannot mutate the typed options directly.
The built-in adapters are exported concrete Source types so callers may create,
embed, compose, or wrap them like external implementations.

Sources that accept command-line flags additionally implement `FlagSource`.
This capability consumes the selected action's semantic configuration tree to
supply flag declarations for parsing and help; `Exec` never detects concrete
source types. All sources receive the full original arguments. A `FlagSource`
also receives the occurrences matched to its own declarations.

A Source control flag that must be recognized before action selection uses the
separate `GlobalFlagSource` capability. Its values are still delivered to that
Source through `SourceInput.Flags`; it does not become a Plugin or an Options
property. `ConfigurationFilesSource` uses this capability for `--config-file`.

After each Source loads, `Exec` compiles every value through the same
semantic configuration tree into ordered property changes. One executor owns
pointer allocation, map updates and deletion, collection replacement, scalar
conversion, unknown-property policy, sensitive diagnostics, and debug logging.
Every successfully applied external value is logged with its SourceValue name
and value whether or not it differs from the current value. Environment
variables therefore log names such as `LISTEN`, command-line values log flags
such as `--mongodb-address`, and configuration files log their file names.
Defaults are not external SourceValues and use the semantic Node path.
Conversion failures are attributed to the current Source and input name without
copying that bootstrap context into every compiled change.

Scalar parsing and display delegate to the reflection module's shared value
conversion. Text interfaces take precedence for command-line and environment
values, `time.Duration` uses Go duration syntax, and `time.Time` uses its
RFC3339 text representation. Structured source syntax remains owned by the
Source and semantic configuration tree.

The default source order is configuration files, environment variables, then
command-line configuration. Nil Sources select this order; an explicit empty
slice disables external configuration. Action defaults remain the mandatory
lowest-priority source and are logged before these replaceable external
Sources. Sources only run after
successful routing and syntax parsing; help never applies a source. A source
failure stops later sources and the action.

Program and per-command settings are fields on their specifications; they do
not use option or builder layers.

## Schema and syntax

Configuration normally reuses the caller's `json` field contract. A caller uses
`config` only when startup configuration needs command-specific semantics, such
as `config:"token,sensitive"`, or a shape that intentionally differs from JSON.
When present, `config` is the complete authoritative contract and `json` is
ignored. Help text uses
the independent `description` tag. Flags are the lower-case canonical path with
dots replaced by hyphens; environment names are the corresponding upper-case
names with underscores. Naming is exact and does not use relaxed matching.

`config:"-"` excludes a field, `config:",inline"` flattens a struct, and
`config:",sensitive"` marks its values as sensitive. `omitempty` belongs to
serialization and is ignored for both `json` and `config` tags. Maps have
string keys.
Structs and maps compile into property changes, slices replace, and null clears
nullable values or deletes map entries. Map flags may use bracketed keys, such as
`labels[region]` and `services[worker]-endpoint`.

Schema compilation uses the runtime value returned by the action's defaults
function. Non-nil pointer fields contribute their nested schema and defaults;
nil pointer fields do not expose configuration inputs. The selected tag's
`omitempty` option never removes fields from the compiled configuration shape.
The semantic reflection `Node` is the only field hierarchy: fixed children are
read directly from `Fields`, and a map or slice's dynamic value shape is read
directly from `Element`. `Exec` retains this root only for the current
execution; canonical paths, sensitivity, field lists, and flag, environment,
and map lookups are derived from the tree where they are used.

Flag declarations use exact long names, an optional short name, or one `{key}`
placeholder. `Exec` compiles Plugin and `GlobalFlagSource` flags globally, then
the selected action's `FlagSource` declarations. It rejects overlap and assigns
occurrences to their owners during the single argument scan. Source order
affects configuration precedence only, not routing or flag ownership.

## Lifecycle and ownership

`Exec` uses only its explicit context, Program, and Execution; it does not
install signals, print returned errors, or exit. `Main` supplies OS arguments,
environment, files, standard streams, signal cancellation, error printing, and
exit status. The Logging Plugin owns default logger injection for both `Main`
and direct `Exec` callers. The first `SIGINT` or `SIGTERM` cancels the
execution context for graceful shutdown; a second signal exits immediately.
The exported `SignalContext` owns this lifecycle for `Main` and other command
entry points. Signal state belongs to that invocation and is not stored
globally.

Command, Plugin, schema, runtime input, Source, patch, and Action errors are
returned by `Exec`. A Program's Plugins and Sources must be reusable and safe
for concurrent executions.

Startup configuration has one authoritative interface in `command`. The
`config` package owns runtime `DynamicConfig`, not executable startup. Reusable
commands for other domains live in adapter packages and return ordinary
`Command` values; the executor has no domain-specific command variants.
