# Common Domain

This context defines reusable domain values shared by common modules.

## Attachments

**Asset**:
A named attachment owned by a target, consisting of content identity, file properties, and caller-defined metadata.
_Avoid_: File, blob

**Blob**:
The content source supplied when storing an Asset, either readable bytes or a direct Link.
_Avoid_: Asset, file

**Metadata**:
Caller-defined string attributes attached to an Asset independently of its content.
_Avoid_: File properties, content metadata

**Content Version**:
The revision of an Asset's bytes or direct Link; replacing Metadata alone does not advance it.
_Avoid_: Metadata version, resource version
