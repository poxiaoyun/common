package container

import "context"

// NerdctlRuntime implements ContainerRuntime through the nerdctl CLI.
type NerdctlRuntime struct {
	*CommandRuntime
}

var _ ContainerRuntime = (*NerdctlRuntime)(nil)

// NerdctlRuntimeProvider detects and opens a nerdctl runtime.
type NerdctlRuntimeProvider struct{}

var _ RuntimeProvider = (*NerdctlRuntimeProvider)(nil)

// NewNerdctlRuntimeProvider creates the built-in nerdctl runtime provider.
func NewNerdctlRuntimeProvider() *NerdctlRuntimeProvider {
	return &NerdctlRuntimeProvider{}
}

// Name implements RuntimeProvider.
func (p *NerdctlRuntimeProvider) Name() string {
	return "nerdctl"
}

// Open implements RuntimeProvider.
func (p *NerdctlRuntimeProvider) Open(ctx context.Context) (ContainerRuntime, error) {
	return OpenCommandRuntime(ctx, p.Name(), "nerdctl", func(executable string) ContainerRuntime {
		return NewNerdctlRuntime(executable)
	})
}

// NewNerdctlRuntime creates a nerdctl CLI adapter.
func NewNerdctlRuntime(executable string) *NerdctlRuntime {
	return &NerdctlRuntime{
		CommandRuntime: NewCommandRuntime("nerdctl", executable),
	}
}
