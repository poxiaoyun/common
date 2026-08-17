// Package mongodb prepares MongoDB clusters for integration tests.
package mongodb

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"xiaoshiai.cn/common/testkit/container"
)

// RequireURI returns MONGODB_URI when configured. When MONGODB_INTEGRATION is
// set, it instead starts a temporary single-node replica set and registers its
// destruction with t. Each temporary cluster has a unique container name and
// randomly allocated host port, so independent calls may run concurrently.
// Otherwise, RequireURI skips the test.
func RequireURI(t testing.TB) string {
	t.Helper()
	if uri := os.Getenv("MONGODB_URI"); uri != "" {
		return uri
	}
	if os.Getenv("MONGODB_INTEGRATION") == "" {
		t.Skip("set MONGODB_INTEGRATION to create a temporary cluster or MONGODB_URI to use an existing cluster")
	}

	runtime, err := container.DiscoverRuntime(t.Context())
	if err != nil {
		t.Fatalf("discover container runtime: %v", err)
	}
	name := "mongodb-it-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	target, err := runtime.CreateContainer(t.Context(), container.ContainerSpec{
		Name:    name,
		Image:   "mongo:8.0",
		Command: []string{"mongod", "--replSet", "rs0", "--bind_ip_all"},
		Ports: []container.PortMapping{
			{
				ContainerPort: container.Port{Number: 27017, Protocol: container.ProtocolTCP},
				HostAddress:   "127.0.0.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("create temporary MongoDB container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runtime.DestroyContainer(ctx, target); err != nil {
			t.Errorf("destroy temporary MongoDB container: %v", err)
		}
	})

	info, err := runtime.InspectContainer(t.Context(), target)
	if err != nil {
		t.Fatalf("inspect temporary MongoDB container: %v", err)
	}
	var binding container.PortBinding
	for _, current := range info.Ports {
		if current.ContainerPort == (container.Port{Number: 27017, Protocol: container.ProtocolTCP}) {
			binding = current
			break
		}
	}
	if binding.HostPort == 0 {
		t.Fatal("temporary MongoDB container has no published port")
	}

	waitForCommand(t, runtime, target, []string{
		"mongosh",
		"--quiet",
		"--eval",
		`rs.initiate({_id:"rs0",members:[{_id:0,host:"localhost:27017"}]})`,
	})
	waitForCommand(t, runtime, target, []string{
		"mongosh",
		"--quiet",
		"--eval",
		`if (!db.hello().isWritablePrimary) { quit(1) }`,
	})
	address := net.JoinHostPort(binding.HostAddress, fmt.Sprint(binding.HostPort))
	return "mongodb://" + address + "/?replicaSet=rs0&directConnection=true"
}

// RequireDatabase connects to a replica set or sharded cluster at uri and
// returns an isolated database. Client options can install codecs required by
// the component under test. The database and client are cleaned up with t.
func RequireDatabase(t testing.TB, uri string, clientOptions ...*mongooptions.ClientOptions) *mongo.Database {
	t.Helper()
	options := append([]*mongooptions.ClientOptions{
		mongooptions.Client().
			ApplyURI(uri).
			SetServerSelectionTimeout(10 * time.Second),
	}, clientOptions...)
	client, err := mongo.Connect(
		t.Context(),
		options...,
	)
	if err != nil {
		t.Fatalf("connect to MongoDB: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnect from MongoDB: %v", err)
		}
	})
	if err := client.Ping(t.Context(), readpref.Primary()); err != nil {
		t.Fatalf("ping MongoDB: %v", err)
	}

	var hello struct {
		SetName string `bson:"setName"`
		Message string `bson:"msg"`
	}
	if err := client.Database("admin").RunCommand(t.Context(), bson.D{
		{Key: "hello", Value: 1},
	}).Decode(&hello); err != nil {
		t.Fatalf("inspect MongoDB topology: %v", err)
	}
	if hello.SetName == "" && hello.Message != "isdbgrid" {
		t.Fatal("MongoDB integration tests require a replica set or sharded cluster")
	}

	database := client.Database("integration_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := database.Drop(ctx); err != nil {
			t.Errorf("drop integration test database: %v", err)
		}
	})
	return database
}

func waitForCommand(t testing.TB, runtime container.ContainerRuntime, target container.Container, command []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		result, err := runtime.Exec(ctx, target, command)
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("container command did not succeed: error=%v", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if result.ExitCode == 0 {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf(
				"container command did not succeed: exitCode=%d, stdout=%s, stderr=%s",
				result.ExitCode,
				result.Stdout,
				result.Stderr,
			)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
