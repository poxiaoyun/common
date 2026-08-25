package container_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
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
				Name:       "database",
				Image:      "example:1",
				Entrypoint: "/usr/local/bin/database",
				Networks:   []string{"cluster"},
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
	if created.Entrypoint != "/usr/local/bin/database" {
		t.Fatalf("container entrypoint = %q", created.Entrypoint)
	}
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

func TestEnvironmentUsesCallerOwnedNetwork(t *testing.T) {
	runtime := &recordingRuntime{}
	environment, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "application-stage",
		Networks: []container.EnvironmentNetworkSpec{
			{Name: "cluster", Network: "shared-network"},
		},
		Containers: []container.EnvironmentContainerSpec{
			{Name: "apps", Image: "apps:1", Networks: []string{"cluster"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if len(runtime.createdSpec.Networks) != 1 || runtime.createdSpec.Networks[0].Network != "shared-network" {
		t.Fatalf("container networks = %#v", runtime.createdSpec.Networks)
	}
	if err := environment.Destroy(t.Context()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create container application-stage-apps",
		"destroy container container-1",
	}) {
		t.Fatalf("operations = %v", runtime.operations)
	}
}

func TestEnvironmentWaitsForExecReadinessBeforeStartingNextContainer(t *testing.T) {
	runtime := &recordingRuntime{}
	_, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "ordered-startup",
		Containers: []container.EnvironmentContainerSpec{
			{
				Name:      "iam",
				Image:     "iam:1",
				Readiness: container.ExecReadinessProbe{Command: []string{"iamctl", "ready"}},
			},
			{Name: "apps", Image: "apps:1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create container ordered-startup-iam",
		"exec container-1 iamctl ready",
		"create container ordered-startup-apps",
	}) {
		t.Fatalf("startup operations = %v", runtime.operations)
	}
}

func TestEnvironmentRetriesExecReadinessUntilItSucceeds(t *testing.T) {
	runtime := &recordingRuntime{execExitCodes: []int{1, 0}}
	_, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "eventual-readiness",
		Containers: []container.EnvironmentContainerSpec{
			{
				Name:      "iam",
				Image:     "iam:1",
				Readiness: container.ExecReadinessProbe{Command: []string{"iamctl", "ready"}},
			},
			{Name: "apps", Image: "apps:1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create container eventual-readiness-iam",
		"exec container-1 iamctl ready",
		"exec container-1 iamctl ready",
		"create container eventual-readiness-apps",
	}) {
		t.Fatalf("startup operations = %v", runtime.operations)
	}
}

func TestEnvironmentStopsReadinessWhenContainerExited(t *testing.T) {
	runtime := &recordingRuntime{containerStates: []container.ContainerState{container.ContainerStateExited}}
	_, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "failed-readiness",
		Containers: []container.EnvironmentContainerSpec{{
			Name:      "mongodb",
			Image:     "mongo:8.0",
			Readiness: container.ExecReadinessProbe{Command: []string{"mongosh", "--eval", "ping"}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "container exited before becoming ready") {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create container failed-readiness-mongodb",
		"inspect container container-1",
		"destroy container container-1",
	}) {
		t.Fatalf("operations = %v", runtime.operations)
	}
}

func TestEnvironmentWaitsForHTTPReadinessOnPublishedPort(t *testing.T) {
	requests := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	host, portValue, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	hostPort, err := strconv.ParseUint(portValue, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{containerInfo: container.ContainerInfo{
		Container: "container-1",
		Name:      "iam",
		State:     container.ContainerStateRunning,
		Ports: []container.PortBinding{{
			ContainerPort: container.Port{Number: 8080, Protocol: container.ProtocolTCP},
			HostAddress:   host,
			HostPort:      uint16(hostPort),
		}},
	}}
	_, err = container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "http-readiness",
		Containers: []container.EnvironmentContainerSpec{
			{
				Name:  "iam",
				Image: "iam:1",
				Ports: []container.PortMapping{{
					ContainerPort: container.Port{Number: 8080, Protocol: container.ProtocolTCP},
				}},
				Readiness: container.HTTPReadinessProbe{
					Port: container.Port{Number: 8080, Protocol: container.ProtocolTCP},
					Path: "/readyz",
				},
			},
			{Name: "apps", Image: "apps:1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("HTTP readiness requests = %d, want 2", requests.Load())
	}
}

func TestEnvironmentWaitsForRunningWhenReadinessIsOmitted(t *testing.T) {
	runtime := &recordingRuntime{containerStates: []container.ContainerState{
		container.ContainerStateCreated,
		container.ContainerStateRunning,
		container.ContainerStateRunning,
	}}
	_, err := container.CreateEnvironment(t.Context(), runtime, container.EnvironmentSpec{
		Name: "running-readiness",
		Containers: []container.EnvironmentContainerSpec{
			{Name: "iam", Image: "iam:1"},
			{Name: "apps", Image: "apps:1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if !reflect.DeepEqual(runtime.operations, []string{
		"create container running-readiness-iam",
		"inspect container container-1",
		"inspect container container-1",
		"create container running-readiness-apps",
		"inspect container container-1",
	}) {
		t.Fatalf("startup operations = %v", runtime.operations)
	}
}

type recordingRuntime struct {
	operations           []string
	createdSpec          container.ContainerSpec
	createContainerError error
	execExitCodes        []int
	containerInfo        container.ContainerInfo
	containerStates      []container.ContainerState
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
	if len(r.containerStates) > 0 {
		r.operations = append(r.operations, "inspect container "+string(target))
		state := r.containerStates[0]
		r.containerStates = r.containerStates[1:]
		return container.ContainerInfo{Container: target, Name: "database", State: state}, nil
	}
	if r.containerInfo.Container != "" {
		return r.containerInfo, nil
	}
	return container.ContainerInfo{Container: target, Name: "database", State: container.ContainerStateRunning}, nil
}

func (r *recordingRuntime) WaitContainer(context.Context, container.Container) (container.ContainerExit, error) {
	return container.ContainerExit{}, nil
}

func (r *recordingRuntime) DestroyContainer(_ context.Context, target container.Container) error {
	r.operations = append(r.operations, "destroy container "+string(target))
	return nil
}

func (r *recordingRuntime) Exec(
	_ context.Context,
	target container.Container,
	command []string,
) (container.ExecResult, error) {
	r.operations = append(r.operations, "exec "+string(target)+" "+strings.Join(command, " "))
	if len(r.execExitCodes) > 0 {
		exitCode := r.execExitCodes[0]
		r.execExitCodes = r.execExitCodes[1:]
		return container.ExecResult{ExitCode: exitCode}, nil
	}
	return container.ExecResult{}, nil
}

func (r *recordingRuntime) Logs(context.Context, container.Container) ([]byte, error) {
	return nil, nil
}
