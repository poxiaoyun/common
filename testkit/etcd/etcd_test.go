package etcd_test

import (
	"context"
	"os"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"xiaoshiai.cn/common/testkit/etcd"
)

func TestRequireEndpointsUsesConfiguredEndpoints(t *testing.T) {
	t.Setenv("ETCD_ENDPOINTS", "http://one.example:2379,http://two.example:2379")

	endpoints := etcd.RequireEndpoints(t, etcd.Options{})
	if len(endpoints) != 2 || endpoints[0] != "http://one.example:2379" || endpoints[1] != "http://two.example:2379" {
		t.Fatalf("RequireEndpoints() = %v", endpoints)
	}
}

func TestIntegrationCluster(t *testing.T) {
	if os.Getenv("ETCD_INTEGRATION") == "" {
		t.Skip("set ETCD_INTEGRATION to run the etcd container integration test")
	}
	endpoints := etcd.RequireEndpoints(t, etcd.Options{})
	client, err := clientv3.New(clientv3.Config{Endpoints: endpoints, DialTimeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("create etcd client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close etcd client: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := client.Put(ctx, "testkit/health", "ready"); err != nil {
		t.Fatalf("write to temporary etcd cluster: %v", err)
	}
}
