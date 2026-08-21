# asset/s3

`New` returns an `asset.Service` backed by an S3-compatible bucket. The bucket
must already exist. Upload policy, bucket, and prefix settings are owned by
`s3.Options`. `Options.Proxy` controls who accesses objects in that bucket:
when false, a caller that prefers a Link may receive a presigned URL; when true,
only the Asset service accesses S3 and callers receive proxied Content. This
setting does not change delivery of external Links stored through `Blob.Link`.
