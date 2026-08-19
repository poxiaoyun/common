package noop_test

import (
	"net/http"
	"testing"

	"xiaoshiai.cn/common/config"
	"xiaoshiai.cn/common/config/noop"
	commonerrors "xiaoshiai.cn/common/errors"
)

func TestDynamicConfigRepresentsDisabledConfigurationCenter(t *testing.T) {
	client := noop.New()
	target := map[string]any{"unchanged": true}
	configuration, err := client.Get(t.Context(), "iam", "server", &target)
	if err != nil || configuration != nil || target["unchanged"] != true {
		t.Fatalf("Get() = %#v, target = %#v, error = %v", configuration, target, err)
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
		if event.Type != config.EventInitial || event.Configuration != nil || event.Error != nil {
			t.Fatalf("initial event = %#v", event)
		}
	case <-t.Context().Done():
		t.Fatal("timed out waiting for initial event")
	}
}
