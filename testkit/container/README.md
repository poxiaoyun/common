# container

`container` provides portable Docker, Podman, and nerdctl operations for local
development and integration-test infrastructure.

Discover a runtime with `DiscoverRuntime`, or construct a specific runtime
provider. `ContainerRuntime` creates and owns networks, volumes, and containers;
callers destroy those resources when finished. `EnvironmentSpec` groups related
resources with logical names and rolls back resources if creation fails.

`WaitContainer` blocks until a container's main process exits. Its
`ContainerExit.ExitCode` contains every exit status, including non-zero status;
an error means the runtime wait itself could not be completed or decoded.
Waiting does not remove the container.

Container specifications may override an image's executable entrypoint while
keeping its command arguments separate. This supports one-shot administration
commands shipped in the same image as a long-running service.

Caller-owned bind mounts may request a private SELinux relabel. This is useful
for read-only test fixtures on SELinux hosts; because the runtime changes the
host path's label, the caller must not share that source with another active
container.

Environment container declarations may configure Exec or HTTP readiness.
Without an explicit probe, Environment waits for the runtime to report the
container as running. Readiness is checked before starting the next declared
container and is bounded by the caller's context.

Service health checks and bootstrap ordering belong to the calling test
harness. `EnvironmentSpec` does not define readiness phases, dependency
conditions, or run-to-completion containers.
