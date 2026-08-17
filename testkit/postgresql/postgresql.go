// Package postgresql prepares PostgreSQL servers for integration tests.
package postgresql

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

// RequireURI returns POSTGRESQL_URI when configured. When
// POSTGRESQL_INTEGRATION is set, it starts a temporary PostgreSQL server.
// Otherwise it skips the test.
func RequireURI(t testing.TB) string {
	t.Helper()
	if uri := os.Getenv("POSTGRESQL_URI"); uri != "" {
		return uri
	}
	if os.Getenv("POSTGRESQL_INTEGRATION") == "" {
		t.Skip("set POSTGRESQL_INTEGRATION to create a temporary server or POSTGRESQL_URI to use an existing server")
	}
	runtime, err := container.DiscoverRuntime(t.Context())
	if err != nil {
		t.Fatalf("discover container runtime: %v", err)
	}
	name := "postgresql-it-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target, err := runtime.CreateContainer(t.Context(), container.ContainerSpec{
		Name:  name,
		Image: "postgres:17",
		Environment: map[string]string{
			"POSTGRES_DB":       "common",
			"POSTGRES_PASSWORD": "common",
		},
		Ports: []container.PortMapping{{
			ContainerPort: container.Port{Number: 5432, Protocol: container.ProtocolTCP},
			HostAddress:   "127.0.0.1",
		}},
	})
	if err != nil {
		t.Fatalf("create temporary PostgreSQL container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runtime.DestroyContainer(ctx, target); err != nil {
			t.Errorf("destroy temporary PostgreSQL container: %v", err)
		}
	})
	binding := PublishedPort(t, runtime, target, 5432)
	WaitForPostgreSQL(t, runtime, target)
	server := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword("postgres", "common"),
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
		t.Fatalf("inspect temporary PostgreSQL container: %v", err)
	}
	for _, binding := range info.Ports {
		if binding.ContainerPort == (container.Port{Number: port, Protocol: container.ProtocolTCP}) {
			return binding
		}
	}
	t.Fatalf("temporary PostgreSQL container has no published port %d", port)
	return container.PortBinding{}
}

// WaitForPostgreSQL waits until the server accepts connections.
func WaitForPostgreSQL(t testing.TB, runtime container.ContainerRuntime, target container.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	for {
		result, err := runtime.Exec(ctx, target, []string{
			"pg_isready", "--host=127.0.0.1", "--username=postgres", "--dbname=common",
		})
		if err == nil && result.ExitCode == 0 {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("PostgreSQL did not become ready: %v", ctx.Err())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
