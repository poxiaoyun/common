package container

import "context"

// DockerRuntime implements ContainerRuntime through the Docker CLI.
type DockerRuntime struct {
	*CommandRuntime
}

var _ ContainerRuntime = (*DockerRuntime)(nil)

// DockerRuntimeProvider detects and opens a Docker runtime.
type DockerRuntimeProvider struct{}

var _ RuntimeProvider = (*DockerRuntimeProvider)(nil)

// NewDockerRuntimeProvider creates the built-in Docker runtime provider.
func NewDockerRuntimeProvider() *DockerRuntimeProvider {
	return &DockerRuntimeProvider{}
}

// Name implements RuntimeProvider.
func (p *DockerRuntimeProvider) Name() string {
	return "docker"
}

// Open implements RuntimeProvider.
func (p *DockerRuntimeProvider) Open(ctx context.Context) (ContainerRuntime, error) {
	return OpenCommandRuntime(ctx, p.Name(), "docker", func(executable string) ContainerRuntime {
		return NewDockerRuntime(executable)
	})
}

// NewDockerRuntime creates a Docker CLI adapter.
func NewDockerRuntime(executable string) *DockerRuntime {
	return &DockerRuntime{
		CommandRuntime: NewCommandRuntime("docker", executable),
	}
}
