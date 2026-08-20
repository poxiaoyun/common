package store_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"xiaoshiai.cn/common/config"
	configstore "xiaoshiai.cn/common/config/store"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/inmemory"
)

type serverConfiguration struct {
	Listen   string         `json:"listen"`
	Features []string       `json:"features,omitempty"`
	Nested   map[string]int `json:"nested,omitempty"`
}

func TestStoredConfigurationUsesConfigurationsResource(t *testing.T) {
	resource, err := store.GetResource(&configstore.StoredConfiguration{})
	if err != nil {
		t.Fatalf("GetResource() error = %v", err)
	}
	if resource != "configurations" {
		t.Fatalf("GetResource() = %q, want configurations", resource)
	}
}

func TestDynamicConfigStoresObjectsAndReturnsEmptyMissingValues(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	want := serverConfiguration{Listen: ":8080", Features: []string{"audit"}}

	created, err := client.Set(t.Context(), "iam", "server", &want)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if created.Name != "server" || created.Version <= 0 || created.Value["listen"] != ":8080" {
		t.Fatalf("Set() = %#v", created)
	}

	got := serverConfiguration{}
	loaded, err := client.Get(t.Context(), "iam", "server", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Listen != want.Listen || len(got.Features) != 1 || got.Features[0] != "audit" {
		t.Fatalf("Get() object = %#v, want %#v", got, want)
	}
	if loaded.Version != created.Version {
		t.Fatalf("Get() Configuration = %#v, want version %d", loaded, created.Version)
	}

	missingTarget := serverConfiguration{Listen: "old", Features: []string{"old"}}
	missing, err := client.Get(t.Context(), "cloud", "server", &missingTarget)
	if err != nil {
		t.Fatalf("Get(other namespace) error = %v", err)
	}
	if missing.Name != "server" || missing.Version != 0 || len(missing.Value) != 0 ||
		missingTarget.Listen != "" || missingTarget.Features != nil || missingTarget.Nested != nil {
		t.Fatalf("Get(other namespace) = %#v, object = %#v", missing, missingTarget)
	}
}

func TestDynamicConfigRejectsNonObjectValues(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	tests := []struct {
		name  string
		value any
	}{
		{name: "null", value: nil},
		{name: "array", value: []string{"one"}},
		{name: "scalar", value: true},
		{name: "invalid JSON", value: json.RawMessage(`{"listen":`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.Set(t.Context(), "iam", "server", test.value); !commonerrors.IsCode(err, http.StatusBadRequest) {
				t.Fatalf("Set() error = %v, want BadRequest", err)
			}
		})
	}
}

func TestDynamicConfigHonorsVersionPreconditions(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	created, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 1}, config.IfVersion(0))
	if err != nil {
		t.Fatalf("Set(version 0) error = %v", err)
	}
	if _, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfVersion(0)); !commonerrors.IsConflict(err) {
		t.Fatalf("Set(second version 0) error = %v, want Conflict", err)
	}
	if _, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfVersion(created.Version+1)); !commonerrors.IsConflict(err) {
		t.Fatalf("Set(stale version) error = %v, want Conflict", err)
	}
	updated, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfVersion(created.Version))
	if err != nil {
		t.Fatalf("Set(current version) error = %v", err)
	}
	if updated.Version <= created.Version {
		t.Fatalf("Set(current version) = %#v", updated)
	}
}

func TestDynamicConfigPatchesExistingAndMissingObjects(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	created, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{
		Listen: ":8080", Features: []string{"first"}, Nested: map[string]int{"a": 1},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	mergedObject := serverConfiguration{}
	merged, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`{"nested":{"b":2}}`),
	}, &mergedObject)
	if err != nil {
		t.Fatalf("Patch(merge) error = %v", err)
	}
	if merged.Name != created.Name || mergedObject.Nested["a"] != 1 || mergedObject.Nested["b"] != 2 {
		t.Fatalf("Patch(merge) = %#v, object = %#v", merged, mergedObject)
	}

	patched, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.JSONPatch,
		Data: json.RawMessage(`[{"op":"replace","path":"/features/0","value":"updated"}]`),
	}, nil, config.IfVersion(merged.Version))
	if err != nil {
		t.Fatalf("Patch(JSON) error = %v", err)
	}
	if patched.Version <= merged.Version {
		t.Fatalf("Patch(JSON) = %#v", patched)
	}
	if _, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`{"stale":true}`),
	}, nil, config.IfVersion(merged.Version)); !commonerrors.IsConflict(err) {
		t.Fatalf("Patch(stale version) error = %v, want Conflict", err)
	}

	createdByPatch, err := client.Patch(t.Context(), "iam", "created-by-patch", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`{"enabled":true}`),
	}, nil, config.IfVersion(0))
	if err != nil || createdByPatch.Version <= 0 || createdByPatch.Value["enabled"] != true {
		t.Fatalf("Patch(missing) = %#v, error = %v", createdByPatch, err)
	}
	createdByJSONPatch, err := client.Patch(t.Context(), "iam", "created-by-json-patch", config.Patch{
		Type: config.JSONPatch,
		Data: json.RawMessage(`[{"op":"add","path":"/enabled","value":true}]`),
	}, nil)
	if err != nil || createdByJSONPatch.Version <= 0 || createdByJSONPatch.Value["enabled"] != true {
		t.Fatalf("Patch(JSON missing) = %#v, error = %v", createdByJSONPatch, err)
	}
	if _, err := client.Patch(t.Context(), "iam", "missing", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`null`),
	}, nil); !commonerrors.IsCode(err, http.StatusBadRequest) {
		t.Fatalf("Patch(non-object result) error = %v, want BadRequest", err)
	}
}

func TestDynamicConfigListsOnlyPersistedKeysByName(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	if _, err := client.Get(t.Context(), "iam", "missing", &map[string]any{}); err != nil {
		t.Fatalf("Get(missing) error = %v", err)
	}
	second, err := client.Set(t.Context(), "iam", "zeta", map[string]any{})
	if err != nil {
		t.Fatalf("Set(zeta) error = %v", err)
	}
	first, err := client.Set(t.Context(), "iam", "alpha", map[string]any{})
	if err != nil {
		t.Fatalf("Set(alpha) error = %v", err)
	}
	keys, err := client.ListKeys(t.Context(), "iam")
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	want := []config.Key{{Name: "alpha", Version: first.Version}, {Name: "zeta", Version: second.Version}}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("ListKeys() = %#v, want %#v", keys, want)
	}
}

func TestDynamicConfigWatchStreamsCurrentSnapshots(t *testing.T) {
	storage := newConfigurationStore(t)
	client := configstore.New(storage)
	if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":8080"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	watcher, err := client.Watch(t.Context(), "iam", "server")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	initial := nextEvent(t, watcher).Configuration
	if initial.Name != "server" || initial.Version <= 0 || initial.Value["listen"] != ":8080" {
		t.Fatalf("initial snapshot = %#v", initial)
	}
	if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":9090"}); err != nil {
		t.Fatalf("Set(update) error = %v", err)
	}
	changed := nextEvent(t, watcher).Configuration
	if changed.Version <= initial.Version || changed.Value["listen"] != ":9090" {
		t.Fatalf("changed snapshot = %#v", changed)
	}

	deleteStoredConfiguration(t, storage, "iam", "server")
	deleted := nextEvent(t, watcher).Configuration
	if deleted.Name != "server" || deleted.Version != 0 || len(deleted.Value) != 0 {
		t.Fatalf("deleted snapshot = %#v", deleted)
	}
}

func TestDynamicConfigWatchStartsWithEmptySnapshot(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	watcher, err := client.Watch(t.Context(), "iam", "missing")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	initial := nextEvent(t, watcher).Configuration
	if initial.Name != "missing" || initial.Version != 0 || len(initial.Value) != 0 {
		t.Fatalf("initial snapshot = %#v", initial)
	}
}

func TestOnChangeDecodesObjectAndVersion(t *testing.T) {
	storage := newConfigurationStore(t)
	client := configstore.New(storage)
	if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":8080"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	objects := []serverConfiguration{}
	versions := []int64{}
	err := config.OnChange(ctx, client, "iam", "server", func(_ context.Context, object serverConfiguration, version int64) error {
		objects = append(objects, object)
		versions = append(versions, version)
		switch len(objects) {
		case 1:
			_, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":9090"})
			return err
		case 2:
			deleteStoredConfiguration(t, storage, "iam", "server")
		case 3:
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("OnChange() error = %v", err)
	}
	if len(objects) != 3 || objects[0].Listen != ":8080" || objects[1].Listen != ":9090" ||
		objects[2].Listen != "" || objects[2].Features != nil || objects[2].Nested != nil {
		t.Fatalf("OnChange() objects = %#v", objects)
	}
	if versions[0] <= 0 || versions[1] <= versions[0] || versions[2] != 0 {
		t.Fatalf("OnChange() versions = %#v", versions)
	}
}

func nextEvent(t *testing.T, watcher config.Watcher) config.Event {
	t.Helper()
	select {
	case event, open := <-watcher.Events():
		if !open {
			t.Fatal("watcher closed")
		}
		if event.Error != nil {
			t.Fatalf("watch event error = %v", event.Error)
		}
		return event
	case <-t.Context().Done():
		t.Fatal("timed out waiting for watch event")
		return config.Event{}
	}
}

func deleteStoredConfiguration(t *testing.T, storage store.Store, namespace, name string) {
	t.Helper()
	target := &store.Unstructured{Object: map[string]any{"id": name, "resource": "configurations"}}
	if err := storage.Scope(store.Scope{Resource: "namespaces", Name: namespace}).Delete(t.Context(), target); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func newConfigurationStore(t *testing.T) store.Store {
	t.Helper()
	schema := store.NewSchema()
	if err := configstore.AddToSchema(schema); err != nil {
		t.Fatalf("AddToSchema() error = %v", err)
	}
	storage, err := inmemory.New(schema)
	if err != nil {
		t.Fatalf("inmemory.New() error = %v", err)
	}
	return storage
}
