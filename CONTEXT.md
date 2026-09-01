# Common Domain

This context defines reusable domain values shared by common modules.

## Authentication

**Credential**:
Secret or cryptographic evidence presented for authentication; it is consumed during verification and is not part of the successful Authentication.
_Avoid_: Subject, identity

**Access Key**:
A public credential identifier paired with a Secret Key; it selects a credential record and does not identify the resulting Subject.
_Avoid_: Username, Subject ID, bearer token

**Secret Key**:
The secret paired with an Access Key and presented, directly or through cryptographic proof, to authenticate possession of that credential.
_Avoid_: Access key, Subject ID

**SSH Public-Key Authentication**:
Authentication that combines a verified proof of SSH private-key possession with the authenticated identity bound to the corresponding public key and SSH username.
_Avoid_: Public-key lookup, SSH username, key fingerprint

**Authentication**:
The successful verification result identifying the effective Subject and, when acting on behalf of another identity, its Actor.
_Avoid_: Credential, HTTP request, login session

**Actor**:
The authenticated principal acting on behalf of the effective Subject.
_Avoid_: Subject type, authentication method

**Subject**:
An authenticated principal identified by its globally unique Subject ID and classified by its immutable Subject Type.
_Avoid_: User, account, PDP entity

**Subject Type**:
The identity-owner-established kind of a Subject; user and anonymous are predefined, while the default kind is used when authentication does not distinguish one.
_Avoid_: PDP entity type, token type, OAuth grant type

**Subject ID**:
The globally unique stable identifier of an authenticated principal, used for authorization, ownership, and audit correlation.
_Avoid_: Username, display name

**Subject Name**:
The provider-verified username or principal name within a Subject Type. The same name may exist in another Type; it may change and is not a stable ownership key.
_Avoid_: Display name, subject ID

**Display Name**:
A human-facing, non-unique label for a subject.
_Avoid_: Subject name, username

## Authorization

**Resource Check**:
A complete proposition asking whether one Subject may perform a domain action on a concrete resource snapshot under supplied context.
_Avoid_: Request gate, role check, access decision

**Access Decision**:
The final Allow or Deny result of one Resource Check.
_Avoid_: NoOpinion, authorization error

**Access Snapshot**:
The opaque authorization state observed by one authorization operation.
_Avoid_: Resource version, cache timestamp

**Resource Reference**:
A concrete Resource Type and Resource ID pair used in authorization facts and paths.
_Avoid_: Display name, untyped resource ID

**Authorization Scope**:
The authorization instance root represented by an empty path, or a root-to-resource path of concrete Resource References.
_Avoid_: OAuth scope, path prefix

**Authorization Policy**:
A Checker-owned decision rule selected by resource type and action over trusted subject, resource, request, and relationship facts.
_Avoid_: Caller input, database query, role name

**Resource Access Constraint**:
A backend-independent boolean condition exactly identifying resources for which one Subject and action are authorized at an Access Snapshot.
_Avoid_: Business filter, resource ID list, database query

**Authorized Resource Query**:
A resource-domain query that combines a Resource Access Constraint with business filtering before sorting, pagination, and counting.
_Avoid_: PDP resource search, post-page filtering

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
