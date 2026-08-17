package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
)

// ErrRuntimeNotFound is returned when no registered container runtime is
// available in the current environment.
var ErrRuntimeNotFound = errors.New("container runtime not found")

// RuntimeProvider detects whether one runtime implementation is available and
// opens it. The provider owns its detection and construction strategy; it is
// not required to use a local command.
type RuntimeProvider interface {
	// Name returns the stable name used to select this provider explicitly.
	Name() string

	// Open checks the current environment and returns a ready runtime. It
	// returns an error when this provider is unavailable or cannot be opened.
	Open(ctx context.Context) (ContainerRuntime, error)
}

// RuntimeRegistry stores runtime providers in discovery order. Its fields are
// private so registration order can only be changed through Register.
type RuntimeRegistry struct {
	mu        sync.RWMutex
	providers []RuntimeProvider
}

// NewRuntimeRegistry creates an empty runtime provider registry.
func NewRuntimeRegistry() *RuntimeRegistry {
	return &RuntimeRegistry{}
}

// Register appends a provider to the discovery order. A registration made
// during discovery is visible to subsequent discoveries.
func (r *RuntimeRegistry) Register(provider RuntimeProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = append(r.providers, provider)
}

// Discover opens the first available provider in registration order. A
// non-empty name restricts discovery to providers with that name.
func (r *RuntimeRegistry) Discover(ctx context.Context, name string) (ContainerRuntime, error) {
	r.mu.RLock()
	providers := append([]RuntimeProvider(nil), r.providers...)
	r.mu.RUnlock()

	var failures error
	for _, provider := range providers {
		if name != "" && provider.Name() != name {
			continue
		}
		runtime, err := provider.Open(ctx)
		if err != nil {
			failures = errors.Join(
				failures,
				fmt.Errorf("open container runtime provider %q: %w", provider.Name(), err),
			)
			continue
		}
		return runtime, nil
	}
	return nil, errors.Join(ErrRuntimeNotFound, failures)
}

// DefaultRuntimeRegistry contains the built-in providers in automatic
// discovery order. Additional providers may be appended with Register.
var DefaultRuntimeRegistry = func() *RuntimeRegistry {
	registry := NewRuntimeRegistry()
	registry.Register(NewPodmanRuntimeProvider())
	registry.Register(NewNerdctlRuntimeProvider())
	registry.Register(NewDockerRuntimeProvider())
	return registry
}()

// DiscoverRuntime discovers a runtime from DefaultRuntimeRegistry.
// TESTKIT_CONTAINER_RUNTIME may select a registered provider by name.
func DiscoverRuntime(ctx context.Context) (ContainerRuntime, error) {
	return DefaultRuntimeRegistry.Discover(ctx, os.Getenv("TESTKIT_CONTAINER_RUNTIME"))
}
