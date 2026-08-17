package container_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/testkit/container"
)

func TestEnvironmentCreatesInspectsAndDestroysResources(t *testing.T) {
	runtime := &recordingRuntime{}
	environment, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "database-test",
		Networks: []container.EnvironmentNetworkSpec{
			{Name: "cluster"},
		},
		Volumes: []container.EnvironmentVolumeSpec{
			{Name: "data"},
			{Name: "fixtures", HostPath: "/fixtures"},
		},
		Containers: []container.EnvironmentContainerSpec{
			{
				Name:     "database",
				Image:    "example:1",
				Networks: []string{"cluster"},
				Mounts: []container.EnvironmentMount{
					{Volume: "data", Target: "/data"},
					{Volume: "fixtures", Target: "/fixtures", ReadOnly: true},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create network database-test-cluster",
		"create volume database-test-data",
		"create container database-test-database",
	}) {
		t.Fatalf("creation operations = %v", runtime.operations)
	}
	created := runtime.createdSpec
	if len(created.Networks) != 1 || created.Networks[0].Network != "network-1" {
		t.Fatalf("container networks = %#v", created.Networks)
	}
	if len(created.Volumes) != 1 || created.Volumes[0].Volume != "volume-1" {
		t.Fatalf("container volumes = %#v", created.Volumes)
	}
	if len(created.Binds) != 1 || created.Binds[0].Source != "/fixtures" {
		t.Fatalf("container binds = %#v", created.Binds)
	}

	info, err := environment.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info.Containers["database"].Container != "container-1" {
		t.Fatalf("container inspection = %#v", info.Containers)
	}
	if info.Networks["cluster"].Network != "network-1" {
		t.Fatalf("network inspection = %#v", info.Networks)
	}

	if err := environment.Destroy(t.Context()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations[len(runtime.operations)-3:], []string{
		"destroy container container-1",
		"destroy volume volume-1",
		"destroy network network-1",
	}) {
		t.Fatalf("destruction operations = %v", runtime.operations)
	}
}

func TestEnvironmentRollsBackCreationFailure(t *testing.T) {
	runtime := &recordingRuntime{
		createContainerError: errors.New("cannot start"),
	}
	_, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "rollback",
		Networks: []container.EnvironmentNetworkSpec{
			{Name: "cluster"},
		},
		Volumes: []container.EnvironmentVolumeSpec{
			{Name: "data"},
		},
		Containers: []container.EnvironmentContainerSpec{
			{
				Name:     "database",
				Image:    "example:1",
				Networks: []string{"cluster"},
				Mounts: []container.EnvironmentMount{
					{Volume: "data", Target: "/data"},
				},
			},
		},
	})
	if !errors.Is(err, runtime.createContainerError) {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create network rollback-cluster",
		"create volume rollback-data",
		"create container rollback-database",
		"destroy volume volume-1",
		"destroy network network-1",
	}) {
		t.Fatalf("rollback operations = %v", runtime.operations)
	}
}

type recordingRuntime struct {
	operations           []string
	createdSpec          container.ContainerSpec
	createContainerError error
}

func (r *recordingRuntime) CreateNetwork(_ context.Context, spec container.NetworkSpec) (container.Network, error) {
	r.operations = append(r.operations, "create network "+spec.Name)
	return "network-1", nil
}

func (r *recordingRuntime) InspectNetwork(
	_ context.Context,
	network container.Network,
) (container.NetworkInfo, error) {
	return container.NetworkInfo{Network: network, Name: "cluster", Driver: "bridge"}, nil
}

func (r *recordingRuntime) DestroyNetwork(_ context.Context, network container.Network) error {
	r.operations = append(r.operations, "destroy network "+string(network))
	return nil
}

func (r *recordingRuntime) CreateVolume(_ context.Context, spec container.VolumeSpec) (container.Volume, error) {
	r.operations = append(r.operations, "create volume "+spec.Name)
	return "volume-1", nil
}

func (r *recordingRuntime) DestroyVolume(_ context.Context, volume container.Volume) error {
	r.operations = append(r.operations, "destroy volume "+string(volume))
	return nil
}

func (r *recordingRuntime) CreateContainer(
	_ context.Context,
	spec container.ContainerSpec,
) (container.Container, error) {
	r.operations = append(r.operations, "create container "+spec.Name)
	r.createdSpec = spec
	if r.createContainerError != nil {
		return "", r.createContainerError
	}
	return "container-1", nil
}

func (r *recordingRuntime) InspectContainer(
	_ context.Context,
	target container.Container,
) (container.ContainerInfo, error) {
	return container.ContainerInfo{Container: target, Name: "database", State: container.ContainerStateRunning}, nil
}

func (r *recordingRuntime) DestroyContainer(_ context.Context, target container.Container) error {
	r.operations = append(r.operations, "destroy container "+string(target))
	return nil
}

func (r *recordingRuntime) Exec(
	context.Context,
	container.Container,
	[]string,
) (container.ExecResult, error) {
	return container.ExecResult{}, nil
}

func (r *recordingRuntime) Logs(context.Context, container.Container) ([]byte, error) {
	return nil, nil
}
