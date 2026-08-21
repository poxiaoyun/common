# OCI Registry Design

## Seam

The package interface is the seam between artifact policy and the OCI
Distribution protocol. It owns endpoint normalization, authentication, TLS,
reference parsing, registry error mapping, content verification, and registry
compatibility behavior. It does not own Helm or other artifact-specific media
types and manifest rules.

The interface uses strings, byte content, and OCI specification descriptors.
It deliberately does not expose go-containerregistry clients, references, or
descriptors so the implementation can change without propagating that change
to callers.

## Invariants

- Endpoints without a scheme use verified HTTPS.
- Bearer token and username/password authentication are mutually exclusive.
- CA and mTLS files are explicit; the module does not scan Docker/containerd
  certificate directories or parse `hosts.toml`.
- Blob and manifest descriptors describe the exact uploaded bytes.
- Downloaded layers are checked against both descriptor size and digest.
- Registry 404 responses map to `ErrNotFound`.
- Removing a tag must not delete a manifest still reachable through another
  tag. Registries without direct tag deletion use a replacement manifest that
  is subsequently deleted by digest.

## Artifact adapters

Artifact-specific modules build their manifests using `PushBlob`,
`PutManifest`, `GetManifest`, and `DownloadLayer`. For example, Apps owns the
Helm config and chart layer media types, the single-chart-layer rule, semantic
version tag normalization, and the HTTP content type exposed to chart clients.
