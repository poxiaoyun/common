package noop_test

import (
	"net/http"
	"testing"

	"xiaoshiai.cn/common/config/noop"
	commonerrors "xiaoshiai.cn/common/errors"
)

func TestDynamicConfigRepresentsDisabledConfigurationCenter(t *testing.T) {
	client := noop.New()
	target := map[string]any{"old": true}
	configuration, err := client.Get(t.Context(), "iam", "server", &target)
	if err != nil || configuration.Name != "server" || configuration.Version != 0 || len(configuration.Value) != 0 || len(target) != 0 {
		t.Fatalf("Get() = %#v, target = %#v, error = %v", configuration, target, err)
	}
	keys, err := client.ListKeys(t.Context(), "iam")
	if err != nil || len(keys) != 0 {
		t.Fatalf("ListKeys() = %#v, error = %v", keys, err)
	}
	if _, err := client.Set(t.Context(), "iam", "server", map[string]any{}); !commonerrors.IsCode(err, http.StatusNotImplemented) {
		t.Fatalf("Set() error = %v, want Unsupported", err)
	}

	watcher, err := client.Watch(t.Context(), "iam", "server")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	select {
	case event := <-watcher.Events():
		current := event.Configuration
		if event.Error != nil || current.Name != "server" || current.Version != 0 || len(current.Value) != 0 {
			t.Fatalf("initial snapshot = %#v", event)
		}
	case <-t.Context().Done():
		t.Fatal("timed out waiting for initial event")
	}
}
