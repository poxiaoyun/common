# OCI Registry

`oci` provides generic OCI Distribution operations behind a Common-owned
interface. Callers identify content with a configured registry endpoint plus a
logical repository and tag or digest; go-containerregistry types remain inside
the package.

```go
client, err := oci.NewClient(oci.ClientOptions{
    Endpoint: "registry.example.com",
    Token:    token,
})
if err != nil {
    return err
}

descriptor, err := client.PushBlob(ctx, "apps/charts/demo", oci.BlobInput{
    MediaType: "application/vnd.example.content.v1+octet-stream",
    Content:   data,
})
```

Endpoints are HTTPS by default. Use an explicit `http://` endpoint for plain
HTTP, or `InsecureSkipTLSVerify` only when certificate verification must be
disabled. `CAFile` names an explicit CA bundle; `CertFile` and `KeyFile` name
an explicit mTLS client identity and must be configured together.

`LoadCertificates` reads those explicit files from any `fs.FS`, including an
embedded or in-memory certificate source.

The OCI Distribution specification does not define a client trust-store
layout, so this module performs no implicit Docker or containerd directory
discovery. Deployments pass the intended files explicitly through `CAFile`,
`CertFile`, and `KeyFile`.

Authentication accepts either a bearer `Token` or a `Username`/`Password`
pair. When neither is configured, access is anonymous. The client does not
implicitly read the Docker credential keychain.

`DownloadLayer` returns a stream plus the complete OCI descriptor. The caller
must close the stream, and must handle size or digest errors returned while
reading it.
