# Common Domain

This context defines reusable domain values shared by common modules.

## Authentication

**Subject ID**:
The stable identifier of an authenticated principal, used for authorization, ownership, and audit correlation.
_Avoid_: Username, display name

**Subject Name**:
The provider-verified username or principal name within an authentication domain. It may change and is not a stable ownership key.
_Avoid_: Display name, subject ID

**Display Name**:
A human-facing, non-unique label for a subject.
_Avoid_: Subject name, username

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
