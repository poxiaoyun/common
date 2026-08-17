// Package mysql prepares MySQL servers for integration tests.
package mysql

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/testkit/container"
)

// RequireURI returns MYSQL_URI when configured. When MYSQL_INTEGRATION is set,
// it starts a temporary MySQL server. Otherwise it skips the test.
func RequireURI(t testing.TB) string {
	t.Helper()
	if uri := os.Getenv("MYSQL_URI"); uri != "" {
		return uri
	}
	if os.Getenv("MYSQL_INTEGRATION") == "" {
		t.Skip("set MYSQL_INTEGRATION to create a temporary server or MYSQL_URI to use an existing server")
	}
	runtime, err := container.DiscoverRuntime(t.Context())
	if err != nil {
		t.Fatalf("discover container runtime: %v", err)
	}
	name := "mysql-it-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target, err := runtime.CreateContainer(t.Context(), container.ContainerSpec{
		Name:  name,
		Image: "mysql:8.4",
		Environment: map[string]string{
			"MYSQL_DATABASE":      "common",
			"MYSQL_ROOT_PASSWORD": "common",
		},
		Ports: []container.PortMapping{{
			ContainerPort: container.Port{Number: 3306, Protocol: container.ProtocolTCP},
			HostAddress:   "127.0.0.1",
		}},
	})
	if err != nil {
		t.Fatalf("create temporary MySQL container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runtime.DestroyContainer(ctx, target); err != nil {
			t.Errorf("destroy temporary MySQL container: %v", err)
		}
	})
	binding := PublishedPort(t, runtime, target, 3306)
	WaitForMySQL(t, runtime, target)
	server := &url.URL{
		Scheme: "mysql",
		User:   url.UserPassword("root", "common"),
		Host:   net.JoinHostPort(binding.HostAddress, fmt.Sprint(binding.HostPort)),
		Path:   "/common",
	}
	return server.String()
}

// PublishedPort returns the TCP binding for port.
func PublishedPort(t testing.TB, runtime container.ContainerRuntime, target container.Container, port uint16) container.PortBinding {
	t.Helper()
	info, err := runtime.InspectContainer(t.Context(), target)
	if err != nil {
		t.Fatalf("inspect temporary MySQL container: %v", err)
	}
	for _, binding := range info.Ports {
		if binding.ContainerPort == (container.Port{Number: port, Protocol: container.ProtocolTCP}) {
			return binding
		}
	}
	t.Fatalf("temporary MySQL container has no published port %d", port)
	return container.PortBinding{}
}

// WaitForMySQL waits until the server accepts authenticated connections.
func WaitForMySQL(t testing.TB, runtime container.ContainerRuntime, target container.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	for {
		result, err := runtime.Exec(ctx, target, []string{
			"mysqladmin", "ping", "--host=127.0.0.1", "--user=root", "--password=common", "--silent",
		})
		if err == nil && result.ExitCode == 0 {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("MySQL did not become ready: %v", ctx.Err())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
