# Container testkit design

## Boundary

The package owns portable mechanics for creating, observing, operating, and
destroying container-runtime resources. Callers own service readiness,
bootstrap sequencing, and application-specific orchestration.

`EnvironmentSpec` is a flat declaration of related resources. It is not a
service dependency graph and does not model task phases or container
completion modes.

An Environment may attach containers to a caller-owned runtime network without
owning its lifecycle. This permits an upper orchestration layer to create
ordered Environment stages around an explicitly awaited one-shot container.

## Container completion

`ContainerRuntime.WaitContainer` waits for the target container's main process
to exit and returns its exit code. A non-zero container exit code is a valid
`ContainerExit` result. Errors report inability to perform or observe the wait,
including context cancellation, runtime command failure, or invalid runtime
output.

Waiting does not stop or remove the container. Callers that use completion for
one-shot work must not configure automatic removal or restart, so one execution
has one observable result.

Docker, Podman, and nerdctl adapters use their shared `wait CONTAINER` command.
Application E2E orchestration interprets exit code zero as successful bootstrap
completion; the runtime lifecycle state remains `exited`.

One-shot orchestration may override an image's executable entrypoint, for
example selecting `iamctl` from the same image that normally starts `iam`.
Command arguments remain separate from that executable override.

Bind mounts remain caller-owned. `SELinuxRelabel` explicitly requests a
private `Z` relabel for a source that otherwise cannot be read by a confined
container. It is opt-in because relabeling changes host filesystem metadata and
makes that source unsuitable for simultaneous sharing.

## Environment readiness

Environment readiness is an availability check performed after a container is
started and before the next declared container is started. It does not add or
change a runtime lifecycle state.

With no explicit probe, Environment waits until inspection reports the
container process as running. An Exec probe waits for a command in the
container to exit with code zero. An HTTP probe resolves the declared
dynamically published container port and waits for a successful HTTP response.
The caller's context bounds the total wait.

Readiness remains independent from one-shot completion. Environment does not
interpret a successful process exit as readiness and does not schedule
completion dependencies.
