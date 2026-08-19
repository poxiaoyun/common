# http

`config/http` implements `config.DynamicConfig` against IAM's Configuration
HTTP routes.

```go
client, err := confighttp.New(ctx, "https://iam.example/v1", token)
```

The address includes the HTTP API prefix. Command callers can use
`config/commandsource.Options` for scheme selection. The adapter maps write conditions to
ETag headers, serializes objects inside the Configuration value envelope and
translates the SSE stream into the contract's Watch events.
