package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

const environmentReadinessInterval = 100 * time.Millisecond

// EnvironmentSpec declares related containers, networks, and shared storage.
// Map keys are logical names local to the environment.
type EnvironmentSpec struct {
	Name       string
	Networks   []EnvironmentNetworkSpec
	Volumes    []EnvironmentVolumeSpec
	Containers []EnvironmentContainerSpec
}

// EnvironmentNetworkSpec declares one environment-owned network.
type EnvironmentNetworkSpec struct {
	Name string
	// Network attaches a caller-owned network when non-empty. Environment does
	// not destroy caller-owned networks.
	Network Network
}

// EnvironmentVolumeSpec declares either environment-owned runtime storage or
// a caller-owned host bind. An empty HostPath declares shared empty storage.
type EnvironmentVolumeSpec struct {
	Name     string
	HostPath string
}

// EnvironmentContainerSpec declares a container using logical environment
// network and volume names.
type EnvironmentContainerSpec struct {
	Name        string
	Image       string
	Entrypoint  string
	Command     []string
	Environment map[string]string
	Networks    []string
	Ports       []PortMapping
	Mounts      []EnvironmentMount
	Binds       []BindMount
	Readiness   ReadinessProbe
}

// ReadinessProbe declares how Environment observes container availability.
type ReadinessProbe interface {
	environmentReadinessProbe()
}

// ExecReadinessProbe observes readiness by executing Command in the container.
type ExecReadinessProbe struct {
	Command []string
}

func (ExecReadinessProbe) environmentReadinessProbe() {}

// HTTPReadinessProbe observes readiness through a published container port.
type HTTPReadinessProbe struct {
	Port Port
	Path string
}

func (HTTPReadinessProbe) environmentReadinessProbe() {}

// EnvironmentMount mounts a declared environment volume into a container.
type EnvironmentMount struct {
	Volume   string
	Target   string
	ReadOnly bool
}

// Environment owns all resources created from an EnvironmentSpec.
type Environment struct {
	runtime    ContainerRuntime
	containers map[string]Container
	networks   map[string]Network
	volumes    map[string]Volume
	order      environmentOrder
}

type environmentOrder struct {
	containers    []string
	networks      []string
	ownedNetworks []string
	volumes       []string
}

// EnvironmentInfo is a current aggregate view keyed by logical resource name.
type EnvironmentInfo struct {
	Containers map[string]ContainerInfo
	Networks   map[string]NetworkInfo
}

// CreateEnvironment validates and interprets spec using runtime. A failed
// creation rolls back every resource created by this operation.
func CreateEnvironment(
	ctx context.Context,
	runtime ContainerRuntime,
	spec EnvironmentSpec,
) (*Environment, error) {
	if err := validateEnvironmentSpec(spec); err != nil {
		return nil, err
	}
	target := &Environment{
		runtime:    runtime,
		containers: make(map[string]Container, len(spec.Containers)),
		networks:   make(map[string]Network, len(spec.Networks)),
		volumes:    make(map[string]Volume, len(spec.Volumes)),
	}
	failed := func(err error) (*Environment, error) {
		return nil, errors.Join(err, target.Destroy(context.WithoutCancel(ctx)))
	}

	for _, declared := range spec.Networks {
		if declared.Network != "" {
			target.networks[declared.Name] = declared.Network
			target.order.networks = append(target.order.networks, declared.Name)
			continue
		}
		network, err := runtime.CreateNetwork(ctx, NetworkSpec{
			Name: resourceName(spec.Name, declared.Name),
		})
		if err != nil {
			return failed(fmt.Errorf("create environment network %q: %w", declared.Name, err))
		}
		target.networks[declared.Name] = network
		target.order.networks = append(target.order.networks, declared.Name)
		target.order.ownedNetworks = append(target.order.ownedNetworks, declared.Name)
	}
	volumes := make(map[string]EnvironmentVolumeSpec, len(spec.Volumes))
	for _, declared := range spec.Volumes {
		volumes[declared.Name] = declared
		if declared.HostPath != "" {
			continue
		}
		volume, err := runtime.CreateVolume(ctx, VolumeSpec{
			Name: resourceName(spec.Name, declared.Name),
		})
		if err != nil {
			return failed(fmt.Errorf("create environment volume %q: %w", declared.Name, err))
		}
		target.volumes[declared.Name] = volume
		target.order.volumes = append(target.order.volumes, declared.Name)
	}
	for _, declared := range spec.Containers {
		containerSpec := ContainerSpec{
			Name:        resourceName(spec.Name, declared.Name),
			Image:       declared.Image,
			Entrypoint:  declared.Entrypoint,
			Command:     declared.Command,
			Environment: declared.Environment,
			Ports:       declared.Ports,
			Binds:       declared.Binds,
		}
		for _, networkName := range declared.Networks {
			containerSpec.Networks = append(containerSpec.Networks, NetworkAttachment{
				Network: target.networks[networkName],
			})
		}
		for _, mount := range declared.Mounts {
			declaredVolume := volumes[mount.Volume]
			if declaredVolume.HostPath != "" {
				containerSpec.Binds = append(containerSpec.Binds, BindMount{
					Source:   declaredVolume.HostPath,
					Target:   mount.Target,
					ReadOnly: mount.ReadOnly,
				})
				continue
			}
			containerSpec.Volumes = append(containerSpec.Volumes, VolumeMount{
				Volume:   target.volumes[mount.Volume],
				Target:   mount.Target,
				ReadOnly: mount.ReadOnly,
			})
		}
		created, err := runtime.CreateContainer(ctx, containerSpec)
		if err != nil {
			return failed(fmt.Errorf("create environment container %q: %w", declared.Name, err))
		}
		target.containers[declared.Name] = created
		target.order.containers = append(target.order.containers, declared.Name)
		if err := waitEnvironmentContainerReady(ctx, runtime, created, declared.Readiness); err != nil {
			return failed(fmt.Errorf("wait for environment container %q readiness: %w", declared.Name, err))
		}
	}
	return target, nil
}

func waitEnvironmentContainerReady(
	ctx context.Context,
	runtime ContainerRuntime,
	target Container,
	probe ReadinessProbe,
) error {
	for {
		var err error
		info, inspectErr := runtime.InspectContainer(ctx, target)
		if inspectErr != nil {
			err = inspectErr
		} else if info.State == ContainerStateExited {
			return errors.New("container exited before becoming ready")
		} else if info.State != ContainerStateRunning {
			err = fmt.Errorf("container state is %q", info.State)
		} else if probe == nil {
			return nil
		} else {
			switch probe := probe.(type) {
			case ExecReadinessProbe:
				result, execErr := runtime.Exec(ctx, target, probe.Command)
				if execErr != nil {
					err = execErr
					break
				}
				if result.ExitCode != 0 {
					err = fmt.Errorf("exec readiness command exited with code %d", result.ExitCode)
				}
			case HTTPReadinessProbe:
				err = probeEnvironmentContainerHTTP(ctx, info, probe)
			default:
				panic("unsupported readiness probe")
			}
		}
		if err != nil {
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), err)
			case <-time.After(environmentReadinessInterval):
				continue
			}
		}
		return nil
	}
}

func probeEnvironmentContainerHTTP(
	ctx context.Context,
	info ContainerInfo,
	probe HTTPReadinessProbe,
) error {
	for _, binding := range info.Ports {
		if binding.ContainerPort != probe.Port {
			continue
		}
		host := binding.HostAddress
		if address := net.ParseIP(host); address != nil && address.IsUnspecified() {
			host = "127.0.0.1"
		}
		endpoint := "http://" + net.JoinHostPort(host, strconv.Itoa(int(binding.HostPort))) + probe.Path
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("HTTP readiness returned status %d", response.StatusCode)
		}
		return nil
	}
	return fmt.Errorf("container port %d/%s is not published", probe.Port.Number, probe.Port.Protocol)
}

// Inspect returns current information for every environment container and
// network, keyed by its logical name.
func (e *Environment) Inspect(ctx context.Context) (EnvironmentInfo, error) {
	info := EnvironmentInfo{
		Containers: make(map[string]ContainerInfo, len(e.containers)),
		Networks:   make(map[string]NetworkInfo, len(e.networks)),
	}
	for _, name := range e.order.containers {
		current, err := e.runtime.InspectContainer(ctx, e.containers[name])
		if err != nil {
			return EnvironmentInfo{}, fmt.Errorf("inspect environment container %q: %w", name, err)
		}
		info.Containers[name] = current
	}
	for _, name := range e.order.networks {
		current, err := e.runtime.InspectNetwork(ctx, e.networks[name])
		if err != nil {
			return EnvironmentInfo{}, fmt.Errorf("inspect environment network %q: %w", name, err)
		}
		info.Networks[name] = current
	}
	return info, nil
}

// Destroy destroys containers, runtime-managed volumes, and networks in
// reverse creation order. It continues cleanup after individual failures.
func (e *Environment) Destroy(ctx context.Context) error {
	var result error
	for index := len(e.order.containers) - 1; index >= 0; index-- {
		name := e.order.containers[index]
		if err := e.runtime.DestroyContainer(ctx, e.containers[name]); err != nil {
			result = errors.Join(result, fmt.Errorf("destroy environment container %q: %w", name, err))
		}
	}
	for index := len(e.order.volumes) - 1; index >= 0; index-- {
		name := e.order.volumes[index]
		if err := e.runtime.DestroyVolume(ctx, e.volumes[name]); err != nil {
			result = errors.Join(result, fmt.Errorf("destroy environment volume %q: %w", name, err))
		}
	}
	for index := len(e.order.ownedNetworks) - 1; index >= 0; index-- {
		name := e.order.ownedNetworks[index]
		if err := e.runtime.DestroyNetwork(ctx, e.networks[name]); err != nil {
			result = errors.Join(result, fmt.Errorf("destroy environment network %q: %w", name, err))
		}
	}
	return result
}

func validateEnvironmentSpec(spec EnvironmentSpec) error {
	if spec.Name == "" {
		return errors.New("environment name is required")
	}
	networks := make(map[string]struct{}, len(spec.Networks))
	for _, declared := range spec.Networks {
		if declared.Name == "" {
			return errors.New("environment network name is required")
		}
		if _, exists := networks[declared.Name]; exists {
			return fmt.Errorf("environment network %q is declared more than once", declared.Name)
		}
		networks[declared.Name] = struct{}{}
	}
	volumes := make(map[string]struct{}, len(spec.Volumes))
	for _, declared := range spec.Volumes {
		if declared.Name == "" {
			return errors.New("environment volume name is required")
		}
		if _, exists := volumes[declared.Name]; exists {
			return fmt.Errorf("environment volume %q is declared more than once", declared.Name)
		}
		volumes[declared.Name] = struct{}{}
	}
	containers := make(map[string]struct{}, len(spec.Containers))
	for _, declared := range spec.Containers {
		if declared.Name == "" {
			return errors.New("environment container name is required")
		}
		if _, exists := containers[declared.Name]; exists {
			return fmt.Errorf("environment container %q is declared more than once", declared.Name)
		}
		containers[declared.Name] = struct{}{}
		for _, network := range declared.Networks {
			if _, exists := networks[network]; !exists {
				return fmt.Errorf("container %q references unknown network %q", declared.Name, network)
			}
		}
		for _, mount := range declared.Mounts {
			if _, exists := volumes[mount.Volume]; !exists {
				return fmt.Errorf("container %q references unknown volume %q", declared.Name, mount.Volume)
			}
		}
	}
	return nil
}

func resourceName(environment string, resource string) string {
	return environment + "-" + resource
}
