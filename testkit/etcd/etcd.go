// Package etcd prepares etcd clusters for integration tests.
package etcd

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/testkit/container"
)

const defaultVersion = "3.6.5"

// Options configures a temporary etcd cluster.
type Options struct {
	// Version selects the etcd image version without the leading "v". An empty
	// version uses 3.6.5.
	Version string
}

// RequireEndpoints returns the comma-separated ETCD_ENDPOINTS when configured.
// When ETCD_INTEGRATION is set, it instead starts a temporary single-node etcd
// cluster and registers its destruction with t. Otherwise, RequireEndpoints
// skips the test.
func RequireEndpoints(t testing.TB, options Options) []string {
	t.Helper()
	if endpoints := os.Getenv("ETCD_ENDPOINTS"); endpoints != "" {
		return strings.Split(endpoints, ",")
	}
	if os.Getenv("ETCD_INTEGRATION") == "" {
		t.Skip("set ETCD_INTEGRATION to create a temporary cluster or ETCD_ENDPOINTS to use an existing cluster")
	}
	version := options.Version
	if version == "" {
		version = defaultVersion
	}

	runtime, err := container.DiscoverRuntime(t.Context())
	if err != nil {
		t.Fatalf("discover container runtime: %v", err)
	}
	name := "etcd-it-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target, err := runtime.CreateContainer(t.Context(), container.ContainerSpec{
		Name:  name,
		Image: "gcr.io/etcd-development/etcd:v" + version,
		Command: []string{
			"/usr/local/bin/etcd",
			"--name", "member",
			"--data-dir", "/etcd-data",
			"--listen-client-urls", "http://0.0.0.0:2379",
			"--advertise-client-urls", "http://0.0.0.0:2379",
			"--listen-peer-urls", "http://0.0.0.0:2380",
			"--initial-advertise-peer-urls", "http://0.0.0.0:2380",
			"--initial-cluster", "member=http://0.0.0.0:2380",
			"--initial-cluster-state", "new",
		},
		Ports: []container.PortMapping{
			{
				ContainerPort: container.Port{Number: 2379, Protocol: container.ProtocolTCP},
				HostAddress:   "127.0.0.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("create temporary etcd container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runtime.DestroyContainer(ctx, target); err != nil {
			t.Errorf("destroy temporary etcd container: %v", err)
		}
	})

	info, err := runtime.InspectContainer(t.Context(), target)
	if err != nil {
		t.Fatalf("inspect temporary etcd container: %v", err)
	}
	var binding container.PortBinding
	for _, current := range info.Ports {
		if current.ContainerPort == (container.Port{Number: 2379, Protocol: container.ProtocolTCP}) {
			binding = current
			break
		}
	}
	if binding.HostPort == 0 {
		t.Fatal("temporary etcd container has no published client port")
	}

	waitForHealth(t, runtime, target)
	address := net.JoinHostPort(binding.HostAddress, fmt.Sprint(binding.HostPort))
	return []string{"http://" + address}
}

func waitForHealth(t testing.TB, runtime container.ContainerRuntime, target container.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := []string{
		"/usr/local/bin/etcdctl",
		"--endpoints=http://127.0.0.1:2379",
		"endpoint",
		"health",
	}
	for {
		result, err := runtime.Exec(ctx, target, command)
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("etcd health check did not succeed: error=%v", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if result.ExitCode == 0 {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf(
				"etcd health check did not succeed: exitCode=%d, stdout=%s, stderr=%s",
				result.ExitCode,
				result.Stdout,
				result.Stderr,
			)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
