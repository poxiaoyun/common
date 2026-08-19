# commandsource

`commandsource` loads one Configuration document into `common/command`.

```go
center := commandsource.New(configClient, "iam", "server")
program.Sources = []command.Source{
    command.ConfigurationFiles(),
    center,
    command.EnvironmentVariables(),
    command.CommandLineArguments(),
}
```

For an `address + token` option, construct the Source directly:

```go
centerOptions := &commandsource.Options{
    Address: "config.example/v1",
    Token:   token,
}
center := commandsource.FromOptions(centerOptions, "iam", "server")
```

To use the standard command sources plus the configuration center:

```go
program.Sources = commandsource.DefaultSources("iam", "server")
```

This adds `--configcenter-address` and `--configcenter-token`. Address
defaults to empty, so the Source uses the Noop adapter and has no effect until
an address is supplied. `CONFIGCENTER_ADDRESS` and
`CONFIGCENTER_TOKEN` provide the same control values without exposing a
token in the process argument list; explicit flags take precedence. A non-empty
address without a scheme uses HTTP.

The stored JSON document uses the same `global` and nested command sections as
a command configuration file. Client construction is explicit and does not
read process environment implicitly.
