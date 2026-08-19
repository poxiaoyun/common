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

func TestDynamicConfigSerializesObjectsAcrossNamespaces(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	want := serverConfiguration{Listen: ":8080", Features: []string{"audit"}}

	created, err := client.Set(t.Context(), "iam", "server", &want)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if created.ID != "server" || created.ResourceVersion <= 0 || !json.Valid(created.Value) {
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
	if loaded.ResourceVersion != created.ResourceVersion {
		t.Fatalf("Get() Configuration = %#v, want version %d", loaded, created.ResourceVersion)
	}

	missingTarget := serverConfiguration{Listen: "unchanged"}
	missing, err := client.Get(t.Context(), "cloud", "server", &missingTarget)
	if err != nil {
		t.Fatalf("Get(other namespace) error = %v", err)
	}
	if missing != nil || missingTarget.Listen != "unchanged" {
		t.Fatalf("Get(other namespace) = %#v, object = %#v", missing, missingTarget)
	}
}

func TestDynamicConfigDistinguishesMissingAndJSONNull(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	if _, err := client.Set(t.Context(), "iam", "optional", nil); err != nil {
		t.Fatalf("Set(null) error = %v", err)
	}
	value := map[string]any{"unchanged": true}
	configuration, err := client.Get(t.Context(), "iam", "optional", &value)
	if err != nil || configuration == nil || value != nil {
		t.Fatalf("Get(null) = %#v, value = %#v, error = %v", configuration, value, err)
	}
}

func TestDynamicConfigRejectsInvalidJSONObjects(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	if _, err := client.Set(t.Context(), "iam", "server", json.RawMessage(`{"listen":`)); !commonerrors.IsCode(err, http.StatusBadRequest) {
		t.Fatalf("Set(invalid JSON) error = %v, want BadRequest", err)
	}
}

func TestDynamicConfigHonorsWritePreconditions(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	created, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 1}, config.IfAbsent())
	if err != nil {
		t.Fatalf("Set(IfAbsent) error = %v", err)
	}
	if _, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfAbsent()); !commonerrors.IsAlreadyExists(err) {
		t.Fatalf("Set(second IfAbsent) error = %v, want AlreadyExists", err)
	}

	stale := created.ResourceVersion + 1
	if _, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfVersion(stale)); !commonerrors.IsConflict(err) {
		t.Fatalf("Set(stale version) error = %v, want Conflict", err)
	}
	updated, err := client.Set(t.Context(), "iam", "server", map[string]int{"revision": 2}, config.IfVersion(created.ResourceVersion))
	if err != nil {
		t.Fatalf("Set(current version) error = %v", err)
	}
	if updated.ResourceVersion <= created.ResourceVersion {
		t.Fatalf("Set(current version) = %#v", updated)
	}
	if _, err := client.Set(t.Context(), "iam", "server", true, config.IfAbsent(), config.IfVersion(updated.ResourceVersion)); !commonerrors.IsCode(err, http.StatusBadRequest) {
		t.Fatalf("Set(conflicting options) error = %v, want BadRequest", err)
	}
}

func TestDynamicConfigPatchesAndOptionallyDecodesResult(t *testing.T) {
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
	if merged.ID != created.ID || mergedObject.Nested["a"] != 1 || mergedObject.Nested["b"] != 2 {
		t.Fatalf("Patch(merge) = %#v, object = %#v", merged, mergedObject)
	}

	patched, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.JSONPatch,
		Data: json.RawMessage(`[{"op":"replace","path":"/features/0","value":"updated"}]`),
	}, nil, config.IfVersion(merged.ResourceVersion))
	if err != nil {
		t.Fatalf("Patch(JSON) error = %v", err)
	}
	if patched.ResourceVersion <= merged.ResourceVersion {
		t.Fatalf("Patch(JSON) = %#v", patched)
	}
	if _, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`{"stale":true}`),
	}, nil, config.IfVersion(merged.ResourceVersion)); !commonerrors.IsConflict(err) {
		t.Fatalf("Patch(stale version) error = %v, want Conflict", err)
	}
	if _, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch, Data: json.RawMessage(`{}`),
	}, nil, config.IfAbsent()); !commonerrors.IsCode(err, http.StatusBadRequest) {
		t.Fatalf("Patch(IfAbsent) error = %v, want BadRequest", err)
	}
}

func TestDynamicConfigWatchStartsWithInitialAndStreamsChanges(t *testing.T) {
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

	initial := nextEvent(t, watcher)
	if initial.Type != config.EventInitial || initial.Configuration == nil || initial.Configuration.ID != "server" {
		t.Fatalf("initial event = %#v", initial)
	}
	if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":9090"}); err != nil {
		t.Fatalf("Set(update) error = %v", err)
	}
	changed := nextEvent(t, watcher)
	if changed.Type != config.EventChange || changed.Configuration == nil || string(changed.Configuration.Value) != `{"listen":":9090"}` {
		t.Fatalf("change event = %#v", changed)
	}

	scoped := storage.Scope(store.Scope{Resource: "namespaces", Name: "iam"})
	if err := scoped.Delete(t.Context(), &config.Configuration{ObjectMeta: store.ObjectMeta{ID: "server"}}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	deleted := nextEvent(t, watcher)
	if deleted.Type != config.EventDelete || deleted.Configuration != nil {
		t.Fatalf("delete event = %#v", deleted)
	}
}

func TestDynamicConfigWatchInitialReportsMissing(t *testing.T) {
	client := configstore.New(newConfigurationStore(t))
	watcher, err := client.Watch(t.Context(), "iam", "missing")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	initial := nextEvent(t, watcher)
	if initial.Type != config.EventInitial || initial.Configuration != nil {
		t.Fatalf("initial missing event = %#v", initial)
	}
}

func TestOnChangeDecodesFreshTypedObjectsAndDeletion(t *testing.T) {
	storage := newConfigurationStore(t)
	client := configstore.New(storage)
	if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":8080"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	objects := []*serverConfiguration{}
	events := []config.EventType{}
	err := config.OnChange[serverConfiguration](ctx, client, "iam", "server", func(_ context.Context, change config.Change[serverConfiguration]) error {
		events = append(events, change.Type)
		objects = append(objects, change.Object)
		switch change.Type {
		case config.EventInitial:
			if _, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":9090"}); err != nil {
				return err
			}
		case config.EventChange:
			scoped := storage.Scope(store.Scope{Resource: "namespaces", Name: "iam"})
			if err := scoped.Delete(t.Context(), &config.Configuration{ObjectMeta: store.ObjectMeta{ID: "server"}}); err != nil {
				return err
			}
		case config.EventDelete:
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("OnChange() error = %v", err)
	}
	if len(events) != 3 || events[0] != config.EventInitial || events[1] != config.EventChange || events[2] != config.EventDelete {
		t.Fatalf("OnChange() events = %#v", events)
	}
	if objects[0] == objects[1] || objects[0].Listen != ":8080" || objects[1].Listen != ":9090" || objects[2] != nil {
		t.Fatalf("OnChange() objects = %#v", objects)
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
