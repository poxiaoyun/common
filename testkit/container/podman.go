package container

import "context"

// PodmanRuntime implements ContainerRuntime through the Podman CLI.
type PodmanRuntime struct {
	*CommandRuntime
}

var _ ContainerRuntime = (*PodmanRuntime)(nil)

// PodmanRuntimeProvider detects and opens a Podman runtime.
type PodmanRuntimeProvider struct{}

var _ RuntimeProvider = (*PodmanRuntimeProvider)(nil)

// NewPodmanRuntimeProvider creates the built-in Podman runtime provider.
func NewPodmanRuntimeProvider() *PodmanRuntimeProvider {
	return &PodmanRuntimeProvider{}
}

// Name implements RuntimeProvider.
func (p *PodmanRuntimeProvider) Name() string {
	return "podman"
}

// Open implements RuntimeProvider.
func (p *PodmanRuntimeProvider) Open(ctx context.Context) (ContainerRuntime, error) {
	return OpenCommandRuntime(ctx, p.Name(), "podman", func(executable string) ContainerRuntime {
		return NewPodmanRuntime(executable)
	})
}

// NewPodmanRuntime creates a Podman CLI adapter.
func NewPodmanRuntime(executable string) *PodmanRuntime {
	return &PodmanRuntime{
		CommandRuntime: NewCommandRuntime("podman", executable),
	}
}
