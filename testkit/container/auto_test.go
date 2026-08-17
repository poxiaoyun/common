package container_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"xiaoshiai.cn/common/testkit/container"
)

func TestDiscoverRuntimeUsesConfiguredBackend(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("get test executable: %v", err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "podman")
	if err := os.Symlink(executable, path); err != nil {
		t.Fatalf("create runtime executable: %v", err)
	}
	t.Setenv("CONTAINER_RUNTIME_HELPER", "1")
	t.Setenv("CONTAINER_RUNTIME_LOG", filepath.Join(t.TempDir(), "commands.log"))
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TESTKIT_CONTAINER_RUNTIME", "podman")

	runtime, err := container.DiscoverRuntime(t.Context())
	if err != nil {
		t.Fatalf("DiscoverRuntime() error = %v", err)
	}
	if _, ok := runtime.(*container.PodmanRuntime); !ok {
		t.Fatalf("DiscoverRuntime() = %T, want *container.PodmanRuntime", runtime)
	}
}

func TestRuntimeRegistryDiscoversInRegistrationOrder(t *testing.T) {
	registry := container.NewRuntimeRegistry()
	want := container.NewCommandRuntime("selected", "unused")
	var opened []string
	registry.Register(runtimeProvider{
		name: "first",
		open: func(context.Context) (container.ContainerRuntime, error) {
			opened = append(opened, "first")
			return nil, errors.New("unavailable")
		},
	})
	registry.Register(runtimeProvider{
		name: "second",
		open: func(context.Context) (container.ContainerRuntime, error) {
			opened = append(opened, "second")
			return want, nil
		},
	})
	registry.Register(runtimeProvider{
		name: "third",
		open: func(context.Context) (container.ContainerRuntime, error) {
			opened = append(opened, "third")
			return nil, errors.New("must not be opened")
		},
	})

	runtime, err := registry.Discover(t.Context(), "")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if runtime != want {
		t.Fatalf("Discover() = %T %p, want %p", runtime, runtime, want)
	}
	if len(opened) != 2 || opened[0] != "first" || opened[1] != "second" {
		t.Fatalf("opened providers = %v, want [first second]", opened)
	}
}

func TestRuntimeRegistrySelectsProviderByName(t *testing.T) {
	registry := container.NewRuntimeRegistry()
	want := container.NewCommandRuntime("selected", "unused")
	registry.Register(runtimeProvider{
		name: "other",
		open: func(context.Context) (container.ContainerRuntime, error) {
			t.Fatal("unselected provider was opened")
			return nil, nil
		},
	})
	registry.Register(runtimeProvider{
		name: "selected",
		open: func(context.Context) (container.ContainerRuntime, error) {
			return want, nil
		},
	})

	runtime, err := registry.Discover(t.Context(), "selected")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if runtime != want {
		t.Fatalf("Discover() = %T %p, want %p", runtime, runtime, want)
	}
}

type runtimeProvider struct {
	name string
	open func(ctx context.Context) (container.ContainerRuntime, error)
}

func (p runtimeProvider) Name() string {
	return p.name
}

func (p runtimeProvider) Open(ctx context.Context) (container.ContainerRuntime, error) {
	return p.open(ctx)
}
