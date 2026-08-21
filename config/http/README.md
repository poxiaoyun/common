# http

`config/http` implements `config.DynamicConfig` against IAM's Configuration
HTTP routes.

```go
client, err := confighttp.New("https://iam.example/v1", token)
```

Services with rotating credentials use `NewWithTransport` to compose their
authentication around the adapter's configured base transport.

The address includes the HTTP API prefix. Command callers can use
`config/commandsource.Options` for scheme selection. The adapter maps version
conditions to ETag headers, serializes JSON objects inside the Configuration
value envelope and translates the response stream into Configuration
snapshots. Transport event kinds are ignored.
