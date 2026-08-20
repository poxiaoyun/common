package http_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaoshiai.cn/common/config"
	confighttp "xiaoshiai.cn/common/config/http"
)

type serverConfiguration struct {
	Listen string `json:"listen"`
}

func TestHTTPDynamicConfigUsesPerCallNamespaceObjectsAndVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/namespaces/iam/configurations/server" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("If-None-Match") != "*" {
				t.Fatalf("PUT If-None-Match = %q", r.Header.Get("If-None-Match"))
			}
			data, _ := io.ReadAll(r.Body)
			if string(data) != `{"value":{"listen":":8080"}}` {
				t.Fatalf("PUT body = %s", data)
			}
			fmt.Fprint(w, `{"name":"server","version":1,"value":{"listen":":8080"}}`)
		case http.MethodPatch:
			if r.Header.Get("Content-Type") != string(config.MergePatch) || r.Header.Get("If-Match") != `"1"` {
				t.Fatalf("PATCH headers = %#v", r.Header)
			}
			fmt.Fprint(w, `{"name":"server","version":2,"value":{"listen":":9090"}}`)
		case http.MethodGet:
			fmt.Fprint(w, `{"name":"server","version":2,"value":{"listen":":9090"}}`)
		}
	}))
	defer server.Close()

	client, err := confighttp.New(t.Context(), server.URL+"/v1", "secret")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	created, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":8080"}, config.IfVersion(0))
	if err != nil || created.Version != 1 {
		t.Fatalf("Set() = %#v, %v", created, err)
	}
	patchedObject := serverConfiguration{}
	patched, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch,
		Data: json.RawMessage(`{"listen":":9090"}`),
	}, &patchedObject, config.IfVersion(created.Version))
	if err != nil || patched.Version != 2 || patchedObject.Listen != ":9090" {
		t.Fatalf("Patch() = %#v, object = %#v, error = %v", patched, patchedObject, err)
	}
	loadedObject := serverConfiguration{}
	loaded, err := client.Get(t.Context(), "iam", "server", &loadedObject)
	if err != nil || loaded.Version != 2 || loadedObject.Listen != ":9090" {
		t.Fatalf("Get() = %#v, object = %#v, error = %v", loaded, loadedObject, err)
	}
}

func TestHTTPDynamicConfigReturnsEmptyMissingValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"status":"Failure","code":404,"reason":"NotFound","message":"configuration not found"}`)
	}))
	defer server.Close()
	client, err := confighttp.New(t.Context(), server.URL, "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	object := serverConfiguration{Listen: "old"}
	current, err := client.Get(t.Context(), "cloud", "server", &object)
	if err != nil || current.Name != "server" || current.Version != 0 || len(current.Value) != 0 || object != (serverConfiguration{}) {
		t.Fatalf("Get(missing) = %#v, object = %#v, error = %v", current, object, err)
	}
}

func TestHTTPDynamicConfigListsKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/namespaces/iam/configurations" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"name":"server","version":5},{"name":"global","version":3}]`)
	}))
	defer server.Close()
	client, err := confighttp.New(t.Context(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := client.ListKeys(t.Context(), "iam")
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	want := []config.Key{{Name: "global", Version: 3}, {Name: "server", Version: 5}}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("ListKeys() = %#v, want %#v", keys, want)
	}
}

func TestHTTPDynamicConfigComposesTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer service-token" {
			t.Fatalf("Authorization = %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"license","version":1,"value":{"license":"content"}}`)
	}))
	defer server.Close()

	client, err := confighttp.NewWithTransport(t.Context(), server.URL, func(base http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			clone := request.Clone(request.Context())
			clone.Header.Set("Authorization", "Bearer service-token")
			return base.RoundTrip(clone)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]string{}
	if _, err := client.Get(t.Context(), "system", "license", &value); err != nil {
		t.Fatal(err)
	}
	if value["license"] != "content" {
		t.Fatalf("value = %#v", value)
	}
}

func TestHTTPDynamicConfigWatchStreamsSnapshotsWithoutEventTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/namespaces/cloud/configurations/server" || r.URL.Query().Get("watch") != "true" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"name\":\"server\",\"version\":1,\"value\":{\"listen\":\":8080\"}}\n\n")
		fmt.Fprint(w, "event: transport-detail\ndata: {\"name\":\"server\",\"version\":2,\"value\":{\"listen\":\":9090\"}}\n\n")
		fmt.Fprint(w, "data: {\"name\":\"server\",\"version\":0,\"value\":{}}\n\n")
	}))
	defer server.Close()
	client, err := confighttp.New(t.Context(), server.URL, "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	watcher, err := client.Watch(t.Context(), "cloud", "server")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer watcher.Stop()
	initial := nextEvent(t, watcher).Configuration
	changed := nextEvent(t, watcher).Configuration
	deleted := nextEvent(t, watcher).Configuration
	if initial.Version != 1 || initial.Value["listen"] != ":8080" {
		t.Fatalf("initial snapshot = %#v", initial)
	}
	if changed.Version != 2 || changed.Value["listen"] != ":9090" {
		t.Fatalf("changed snapshot = %#v", changed)
	}
	if deleted.Name != "server" || deleted.Version != 0 || len(deleted.Value) != 0 {
		t.Fatalf("deleted snapshot = %#v", deleted)
	}
}

func TestHTTPDynamicConfigWatchRequiresInitialSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer server.Close()
	client, err := confighttp.New(t.Context(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	watcher, err := client.Watch(t.Context(), "cloud", "server")
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()
	select {
	case event := <-watcher.Events():
		if event.Error == nil || !strings.Contains(event.Error.Error(), "before its initial snapshot") {
			t.Fatalf("terminal event = %#v", event)
		}
	case <-t.Context().Done():
		t.Fatal("timed out waiting for protocol error")
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
