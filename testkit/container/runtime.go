package container

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// CommandRuntime implements the Docker-compatible CLI subset shared by the
// supported runtime adapters. Its fields are private so adapters must use
// NewCommandRuntime and cannot create partially configured values.
type CommandRuntime struct {
	name       string
	executable string
}

var _ ContainerRuntime = (*CommandRuntime)(nil)

// NewCommandRuntime creates a Docker-compatible command runtime.
func NewCommandRuntime(name string, executable string) *CommandRuntime {
	return &CommandRuntime{
		name:       name,
		executable: executable,
	}
}

// OpenCommandRuntime checks a Docker-compatible command and constructs its
// runtime adapter. Command-backed providers may share this implementation;
// RuntimeProvider itself does not require command-based discovery.
func OpenCommandRuntime(
	ctx context.Context,
	name string,
	executable string,
	create func(executable string) ContainerRuntime,
) (ContainerRuntime, error) {
	path, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("find %s executable %q: %w", name, executable, err)
	}
	output, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run %s version: %w: %s", name, err, output)
	}
	return create(path), nil
}

// CreateNetwork implements ContainerRuntime.
func (r *CommandRuntime) CreateNetwork(ctx context.Context, spec NetworkSpec) (Network, error) {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"network",
		"create",
		"--driver",
		"bridge",
		spec.Name,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s network create: %w: %s", r.name, err, output)
	}
	return Network(strings.TrimSpace(string(output))), nil
}

// InspectNetwork implements ContainerRuntime.
func (r *CommandRuntime) InspectNetwork(ctx context.Context, network Network) (NetworkInfo, error) {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"network",
		"inspect",
		"--format",
		"{{json .}}",
		string(network),
	).CombinedOutput()
	if err != nil {
		return NetworkInfo{}, fmt.Errorf("%s network inspect: %w: %s", r.name, err, output)
	}
	var inspected struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Driver string `json:"Driver"`
	}
	if err := json.Unmarshal(output, &inspected); err != nil {
		return NetworkInfo{}, fmt.Errorf("decode %s network inspection: %w", r.name, err)
	}
	return NetworkInfo{
		Network: Network(inspected.ID),
		Name:    inspected.Name,
		Driver:  inspected.Driver,
	}, nil
}

// DestroyNetwork implements ContainerRuntime.
func (r *CommandRuntime) DestroyNetwork(ctx context.Context, network Network) error {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"network",
		"rm",
		string(network),
	).CombinedOutput()
	if err != nil {
		if commandResourceMissing(output) {
			return nil
		}
		return fmt.Errorf("%s network rm: %w: %s", r.name, err, output)
	}
	return nil
}

// CreateVolume implements ContainerRuntime.
func (r *CommandRuntime) CreateVolume(ctx context.Context, spec VolumeSpec) (Volume, error) {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"volume",
		"create",
		"--name",
		spec.Name,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s volume create: %w: %s", r.name, err, output)
	}
	return Volume(strings.TrimSpace(string(output))), nil
}

// DestroyVolume implements ContainerRuntime.
func (r *CommandRuntime) DestroyVolume(ctx context.Context, volume Volume) error {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"volume",
		"rm",
		string(volume),
	).CombinedOutput()
	if err != nil {
		if commandResourceMissing(output) {
			return nil
		}
		return fmt.Errorf("%s volume rm: %w: %s", r.name, err, output)
	}
	return nil
}

// CreateContainer implements ContainerRuntime.
func (r *CommandRuntime) CreateContainer(ctx context.Context, spec ContainerSpec) (Container, error) {
	args := []string{"run", "--detach", "--name", spec.Name}
	for _, name := range sortedDockerEnvironment(spec.Environment) {
		args = append(args, "--env", name+"="+spec.Environment[name])
	}
	for _, attachment := range spec.Networks {
		args = append(args, "--network", string(attachment.Network))
	}
	for _, mapping := range spec.Ports {
		args = append(args, "--publish", commandPortMapping(mapping))
	}
	for _, mount := range spec.Volumes {
		value := "type=volume,src=" + string(mount.Volume) + ",dst=" + mount.Target
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	for _, mount := range spec.Binds {
		value := "type=bind,src=" + mount.Source + ",dst=" + mount.Target
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	process := exec.CommandContext(ctx, r.executable, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	err := process.Run()
	if err != nil {
		_, _ = exec.CommandContext(
			context.WithoutCancel(ctx),
			r.executable,
			"rm",
			"--force",
			spec.Name,
		).CombinedOutput()
		return "", fmt.Errorf("%s run: %w: %s", r.name, err, stderr.Bytes())
	}
	return Container(strings.TrimSpace(stdout.String())), nil
}

// InspectContainer implements ContainerRuntime.
func (r *CommandRuntime) InspectContainer(ctx context.Context, target Container) (ContainerInfo, error) {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"inspect",
		"--format",
		"{{json .}}",
		string(target),
	).CombinedOutput()
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("%s inspect: %w: %s", r.name, err, output)
	}
	return decodeCommandContainer(r.name, output)
}

// DestroyContainer implements ContainerRuntime.
func (r *CommandRuntime) DestroyContainer(ctx context.Context, target Container) error {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"rm",
		"--force",
		string(target),
	).CombinedOutput()
	if err != nil {
		if commandResourceMissing(output) {
			return nil
		}
		return fmt.Errorf("%s rm: %w: %s", r.name, err, output)
	}
	return nil
}

// Exec implements ContainerRuntime.
func (r *CommandRuntime) Exec(
	ctx context.Context,
	target Container,
	command []string,
) (ExecResult, error) {
	args := append([]string{"exec", string(target)}, command...)
	process := exec.CommandContext(ctx, r.executable, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	err := process.Run()
	result := ExecResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if err != nil {
		if ctx.Err() != nil {
			return ExecResult{}, ctx.Err()
		}
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return ExecResult{}, fmt.Errorf("%s exec: %w", r.name, err)
		}
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return result, nil
}

// Logs implements ContainerRuntime.
func (r *CommandRuntime) Logs(ctx context.Context, target Container) ([]byte, error) {
	output, err := exec.CommandContext(
		ctx,
		r.executable,
		"logs",
		string(target),
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s logs: %w: %s", r.name, err, output)
	}
	return output, nil
}

func decodeCommandContainer(runtimeName string, value []byte) (ContainerInfo, error) {
	var inspected struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostAddress string `json:"HostIp"`
				HostPort    string `json:"HostPort"`
			} `json:"Ports"`
			Networks map[string]struct {
				NetworkID string `json:"NetworkID"`
				Address   string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(value, &inspected); err != nil {
		return ContainerInfo{}, fmt.Errorf("decode %s container inspection: %w", runtimeName, err)
	}
	info := ContainerInfo{
		Container: Container(inspected.ID),
		Name:      strings.TrimPrefix(inspected.Name, "/"),
		State:     ContainerState(inspected.State.Status),
	}
	for value, bindings := range inspected.NetworkSettings.Ports {
		port, err := commandPort(value)
		if err != nil {
			return ContainerInfo{}, err
		}
		for _, binding := range bindings {
			hostPort, err := strconv.ParseUint(binding.HostPort, 10, 16)
			if err != nil {
				return ContainerInfo{}, fmt.Errorf("decode %s host port %q: %w", runtimeName, binding.HostPort, err)
			}
			info.Ports = append(info.Ports, PortBinding{
				ContainerPort: port,
				HostAddress:   binding.HostAddress,
				HostPort:      uint16(hostPort),
			})
		}
	}
	for _, attached := range inspected.NetworkSettings.Networks {
		info.Networks = append(info.Networks, ContainerNetwork{
			Network: Network(attached.NetworkID),
			Address: attached.Address,
		})
	}
	sort.Slice(info.Ports, func(first int, second int) bool {
		if info.Ports[first].ContainerPort.Number != info.Ports[second].ContainerPort.Number {
			return info.Ports[first].ContainerPort.Number < info.Ports[second].ContainerPort.Number
		}
		return info.Ports[first].HostAddress < info.Ports[second].HostAddress
	})
	sort.Slice(info.Networks, func(first int, second int) bool {
		return info.Networks[first].Network < info.Networks[second].Network
	})
	return info, nil
}

func commandPort(value string) (Port, error) {
	parts := strings.Split(value, "/")
	number, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return Port{}, fmt.Errorf("decode container port %q: %w", value, err)
	}
	return Port{
		Number:   uint16(number),
		Protocol: Protocol(parts[1]),
	}, nil
}

func commandPortMapping(mapping PortMapping) string {
	containerPort := strconv.Itoa(int(mapping.ContainerPort.Number)) + "/" + string(mapping.ContainerPort.Protocol)
	hostPort := ""
	if mapping.HostPort != 0 {
		hostPort = strconv.Itoa(int(mapping.HostPort))
	}
	if mapping.HostAddress == "" {
		return hostPort + ":" + containerPort
	}
	return mapping.HostAddress + ":" + hostPort + ":" + containerPort
}

func sortedDockerEnvironment(environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func commandResourceMissing(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "no such container") ||
		strings.Contains(message, "no such network") ||
		strings.Contains(message, "no such volume") ||
		strings.Contains(message, "not found")
}
