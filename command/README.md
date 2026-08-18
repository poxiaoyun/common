# command

`command` defines and runs complete executable programs: command routing, typed
startup configuration, help, streams, and process lifecycle.

```go
program := command.Program{
    Command: command.Command{
        Name:    "tool",
        Summary: "Administration tool",
        Children: []command.Command{{
            Name: "serve",
            Options: func() any {
                options := NewDefaultOptions()
                return &options
            },
            Run: func(invocation command.Invocation) error {
                return Run(invocation, command.Options[Options](invocation))
            },
        }},
    },
}

command.Main(program)
```

`Program` and `Command` are specifications. `Exec` interprets the specification
for one execution. `Command.Options` returns fresh defaults; `Exec` applies
external Sources from low to high priority and exposes the result through
`command.Options[T](Invocation)` before calling `Run`. Defaults are the mandatory
lowest-priority source; external Sources default to discovered or explicitly
selected files, environment variables, and command-line configuration.

`Main` connects the process lifecycle: the first `SIGINT` or `SIGTERM` cancels
`Invocation.Context` for graceful shutdown, while a second signal exits
immediately. Direct `Exec` callers retain ownership of their context policy and
may use `SignalContext` for the same behavior.

A Command may be both executable and contain children. Until the current action
receives a positional argument, an action flag, or `--`, an exact child name
selects that child. After the action is selected, all child names are ordinary
positional values.

## Plugins

Plugins provide command behavior that is not configuration. Nil `Program.Plugins`
selects the logging, version, help, and command-suggestion Plugins by default:

- `Logging` places the current or default logger in the execution context and
  registers `-v` and `--v` before any Source runs.
- `Version` registers `--version` and contributes the `version` command.
- `Help` registers `-h` and `--help` and renders the selected `CommandRuntime`.
- `CommandSuggestions` enriches structured unknown-command errors.

`CommandPlugin` contributes root commands before routing. `FlagPlugin` owns
global flags and handles its parsed values. A `CommandErrorPlugin` transforms
parsing and routing errors. All flags are compiled for each execution and
parsed in one pass before Plugins, Sources, and the Action run. An explicit
empty Plugin slice disables the defaults.

## Sources

Every configuration input implements the same interface:

```go
type Source interface {
    Name() string
    Load(context.Context, SourceInput) ([]SourceValue, error)
}
```

A source that owns options also implements `FlagSource`, which derives its
declarations from the selected action's semantic configuration tree. Each
source sees the complete arguments, environment, selected command path, file
reader, and configuration tree. `Exec` supplies the flag occurrences belonging
to that source without hiding the original arguments.

A Source control option that must work before action selection implements
`GlobalFlagSource`. `ConfigurationFilesSource` uses it for `--config-file`, so
the option is accepted both before and after a subcommand.

Sources return named values containing nested maps or tagged structs. `Exec`
compiles every value into property changes and applies all changes
through one typed configuration path. This centralizes source precedence,
unknown-property handling, scalar conversion, sensitive diagnostics, and debug
logs for every applied value. Logs retain the established `config` event with
only `from`, `key`, and `val` fields. Environment variables and command-line
options retain their source-native keys, such as `LISTEN` and
`--mongodb-address`; configuration files use their file names. Defaults use the
semantic path formed from configuration Node names. Sensitive values are redacted.
Text-capable values use their standard text representation in input, help, and
logs. In particular, durations use Go duration syntax such as `30s`, and times
use RFC3339.

Custom sources may be placed anywhere in precedence order:

```go
program.Sources = []command.Source{
    command.ConfigurationFiles(),
    configCenter,
    command.EnvironmentVariables(),
    command.CommandLineArguments(),
}
```

The built-ins are ordinary exported implementations:
`ConfigurationFilesSource`, `EnvironmentVariablesSource`, and
`CommandLineArgumentsSource`. The constructor functions return those concrete
types rather than hiding them behind `Source`.

Nil Sources select the defaults. An explicit empty slice disables external
configuration. Plugins follow the same nil-versus-empty rule.

## Configuration

Configuration normally uses the existing `json` tag so files, environment
variables, flags, and JSON serialization share one field vocabulary. Callers
generally should not add a `config` tag when `json` already expresses the
required name and shape.

Use `config` only for command-specific behavior or when startup configuration
intentionally differs from JSON. It takes complete precedence over `json`:
when `config` is present, its name and options are authoritative and `json` is
ignored. For example, `json:"token,omitempty" config:"token,sensitive"`
redacts the value. `config:"-"` excludes a runtime-only field, and
`config:",inline"` flattens a struct. Help text uses the independent
`description:"Help text"` tag. The serialization-specific `omitempty` option
is ignored in both `json` and `config` tags.

The value returned by the defaults function defines both displayed defaults
and the enabled configuration shape. A nil pointer field is excluded from
files, environment variables, and command-line options; initialize the pointer
in the defaults function to make that configuration branch available.
Empty strings, booleans, maps, and collections remain configurable regardless
of `omitempty`.

Files are discovered in this low-to-high order:

```text
config/<executable>.yaml
config/<executable>.json
<executable>.yaml
<executable>.json
```

`--config-file` is global, overrides `CONFIG_FILE`, and disables discovery. Nested actions
read the object at their command path. Structs and maps merge recursively,
slices replace, and null clears nullable fields or deletes map keys.

CLI configuration accepts `--name=value`, `--name value`, declared short
options, bare booleans, and map keys such as `--labels[region]=west`.
Positional arguments are declared in order on the action:

```go
Arguments: []command.Argument{
    {Name: "source", Summary: "Source file"},
    {Name: "destination", Summary: "Destination file", Optional: true},
},
```

An ordinary argument accepts exactly one value, `Optional` accepts zero or one,
`Variadic` accepts one or more, and both together accept zero or more. A
variadic argument must be last, and an optional argument cannot precede a
required argument. Actions with no declarations reject positional arguments.
Parsed values remain available in declaration order through
`Invocation.Arguments`; `--` ends option parsing and sends all following words
there unchanged. `Invocation.Context` and `Invocation.Streams` expose the
remaining runtime inputs explicitly.
