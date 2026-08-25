// Package container defines portable container runtime operations used to
// prepare local development and integration-test environments.
package container

import "context"

// Container identifies a container inside the runtime that created it.
type Container string

// Network identifies a network inside the runtime that created it.
type Network string

// Volume identifies runtime-managed storage inside the runtime that created it.
type Volume string

// ContainerRuntime creates and operates container resources. Resource handles
// returned by one runtime must only be passed back to that runtime.
type ContainerRuntime interface {
	// CreateNetwork creates a network and transfers ownership to the caller.
	CreateNetwork(ctx context.Context, spec NetworkSpec) (Network, error)

	// InspectNetwork returns the current standardized network information.
	InspectNetwork(ctx context.Context, network Network) (NetworkInfo, error)

	// DestroyNetwork destroys a network. Repeated calls after successful
	// destruction return nil. A network still in use must not be destroyed.
	DestroyNetwork(ctx context.Context, network Network) error

	// CreateVolume creates runtime-managed storage and transfers ownership to
	// the caller. The same volume may be mounted by multiple containers.
	CreateVolume(ctx context.Context, spec VolumeSpec) (Volume, error)

	// DestroyVolume destroys runtime-managed storage. Repeated calls after
	// successful destruction return nil. A mounted volume must not be destroyed.
	DestroyVolume(ctx context.Context, volume Volume) error

	// CreateContainer creates and starts a container. If creation fails, the
	// implementation removes resources partially created by this operation.
	CreateContainer(ctx context.Context, spec ContainerSpec) (Container, error)

	// InspectContainer returns the current standardized container information.
	InspectContainer(ctx context.Context, target Container) (ContainerInfo, error)

	// WaitContainer waits for the container process to exit. Every process exit
	// code, including a non-zero code, is returned in ContainerExit.
	WaitContainer(ctx context.Context, target Container) (ContainerExit, error)

	// DestroyContainer force-stops and removes a container. Repeated calls after
	// successful destruction return nil.
	DestroyContainer(ctx context.Context, target Container) error

	// Exec executes a command in a running container and waits for completion.
	// A non-zero command status is returned in ExecResult rather than as an error.
	Exec(ctx context.Context, target Container, command []string) (ExecResult, error)

	// Logs returns the container logs currently available and does not follow
	// future output.
	Logs(ctx context.Context, target Container) ([]byte, error)
}

// NetworkSpec describes a network to create.
type NetworkSpec struct {
	Name string
}

// NetworkInfo is standardized network information returned by a runtime.
type NetworkInfo struct {
	Network Network
	Name    string
	Driver  string
}

// VolumeSpec describes runtime-managed storage to create.
type VolumeSpec struct {
	Name string
}

// ContainerSpec describes a container to create and start.
type ContainerSpec struct {
	Name        string
	Image       string
	Entrypoint  string
	Command     []string
	Environment map[string]string
	Networks    []NetworkAttachment
	Ports       []PortMapping
	Volumes     []VolumeMount
	Binds       []BindMount
}

// NetworkAttachment connects a container to a runtime network.
type NetworkAttachment struct {
	Network Network
}

// VolumeMount mounts runtime-managed storage into a container.
type VolumeMount struct {
	Volume   Volume
	Target   string
	ReadOnly bool
}

// BindMount mounts a caller-owned host path into a container. The runtime does
// not create, modify permissions for, or remove Source.
type BindMount struct {
	Source         string
	Target         string
	ReadOnly       bool
	// SELinuxRelabel gives Source a private container label before mounting it.
	SELinuxRelabel bool
}

// Protocol identifies a published transport protocol.
type Protocol string

const (
	// ProtocolTCP publishes a TCP port.
	ProtocolTCP Protocol = "tcp"

	// ProtocolUDP publishes a UDP port.
	ProtocolUDP Protocol = "udp"
)

// Port identifies a container port and transport protocol.
type Port struct {
	Number   uint16
	Protocol Protocol
}

// PortMapping requests a host binding. HostPort zero asks the runtime to
// allocate a free port.
type PortMapping struct {
	ContainerPort Port
	HostAddress   string
	HostPort      uint16
}

// PortBinding is an actual host binding reported by container inspection.
type PortBinding struct {
	ContainerPort Port
	HostAddress   string
	HostPort      uint16
}

// ContainerState is the runtime-reported container lifecycle state.
type ContainerState string

const (
	// ContainerStateCreated means the container exists but has not started.
	ContainerStateCreated ContainerState = "created"

	// ContainerStateRunning means the container process is running.
	ContainerStateRunning ContainerState = "running"

	// ContainerStateExited means the container process has exited.
	ContainerStateExited ContainerState = "exited"
)

// ContainerNetwork describes one current network attachment.
type ContainerNetwork struct {
	Network Network
	Address string
}

// ContainerInfo is standardized container information returned by a runtime.
type ContainerInfo struct {
	Container Container
	Name      string
	State     ContainerState
	Ports     []PortBinding
	Networks  []ContainerNetwork
}

// ContainerExit is the completed result of a container's main process.
type ContainerExit struct {
	ExitCode int
}

// ExecResult is the completed result of a command executed in a container.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}
