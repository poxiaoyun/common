# S3 adapter design

Each asset is one object below `<prefix>/<kind>/<target>/<asset>`. Logical
version, digest, user metadata, and timestamps use stable `asset-*` S3 metadata
keys. When `Options.Proxy` is false, `Resolve` returns a presigned link when
requested and otherwise returns the object body directly. When Proxy is true,
the configured S3 endpoint is treated as service-only: `Resolve` uses GetObject
and returns Content even when the caller prefers a Link. A Link-backed Asset
stores its external direct Link in object metadata and uses the S3 object only
as its persistence record, so Proxy does not change that Link's delivery.
