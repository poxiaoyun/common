package http_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/config"
	confighttp "xiaoshiai.cn/common/config/http"
)

type serverConfiguration struct {
	Listen string `json:"listen"`
}

func TestHTTPDynamicConfigUsesPerCallNamespaceObjectsAndConditions(t *testing.T) {
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
			fmt.Fprint(w, `{"id":"server","resourceVersion":1,"value":{"listen":":8080"}}`)
		case http.MethodPatch:
			if r.Header.Get("Content-Type") != string(config.MergePatch) || r.Header.Get("If-Match") != `"1"` {
				t.Fatalf("PATCH headers = %#v", r.Header)
			}
			fmt.Fprint(w, `{"id":"server","resourceVersion":2,"value":{"listen":":9090"}}`)
		case http.MethodGet:
			fmt.Fprint(w, `{"id":"server","resourceVersion":2,"value":{"listen":":9090"}}`)
		}
	}))
	defer server.Close()

	client, err := confighttp.New(t.Context(), server.URL+"/v1", "secret")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	created, err := client.Set(t.Context(), "iam", "server", &serverConfiguration{Listen: ":8080"}, config.IfAbsent())
	if err != nil || created.ResourceVersion != 1 {
		t.Fatalf("Set() = %#v, %v", created, err)
	}
	patchedObject := serverConfiguration{}
	patched, err := client.Patch(t.Context(), "iam", "server", config.Patch{
		Type: config.MergePatch, Data: json.RawMessage(`{"listen":":9090"}`),
	}, &patchedObject, config.IfVersion(created.ResourceVersion))
	if err != nil || patched.ResourceVersion != 2 || patchedObject.Listen != ":9090" {
		t.Fatalf("Patch() = %#v, object = %#v, error = %v", patched, patchedObject, err)
	}
	loadedObject := serverConfiguration{}
	loaded, err := client.Get(t.Context(), "iam", "server", &loadedObject)
	if err != nil || loaded.ResourceVersion != 2 || loadedObject.Listen != ":9090" {
		t.Fatalf("Get() = %#v, object = %#v, error = %v", loaded, loadedObject, err)
	}
}

func TestHTTPDynamicConfigTreatsNotFoundAsEmpty(t *testing.T) {
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
	object := serverConfiguration{Listen: "unchanged"}
	configuration, err := client.Get(t.Context(), "cloud", "server", &object)
	if err != nil || configuration != nil || object.Listen != "unchanged" {
		t.Fatalf("Get(missing) = %#v, object = %#v, error = %v", configuration, object, err)
	}
}

func TestHTTPDynamicConfigWatchStreamsInitialChangeAndDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/namespaces/cloud/configurations/server" || r.URL.Query().Get("watch") != "true" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: initial\ndata: {\"configuration\":{\"id\":\"server\",\"resourceVersion\":1,\"value\":{\"listen\":\":8080\"}}}\n\n")
		fmt.Fprint(w, "event: change\ndata: {\"configuration\":{\"id\":\"server\",\"resourceVersion\":2,\"value\":{\"listen\":\":9090\"}}}\n\n")
		fmt.Fprint(w, "event: delete\ndata: {}\n\n")
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
	initial := nextEvent(t, watcher)
	changed := nextEvent(t, watcher)
	deleted := nextEvent(t, watcher)
	if initial.Type != config.EventInitial || initial.Configuration == nil || initial.Configuration.ResourceVersion != 1 {
		t.Fatalf("initial event = %#v", initial)
	}
	if changed.Type != config.EventChange || changed.Configuration == nil || changed.Configuration.ResourceVersion != 2 {
		t.Fatalf("change event = %#v", changed)
	}
	if deleted.Type != config.EventDelete || deleted.Configuration != nil {
		t.Fatalf("delete event = %#v", deleted)
	}
}

func TestHTTPDynamicConfigWatchRejectsInvalidInitialSequence(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream string
	}{
		{name: "change first", stream: "event: change\ndata: {}\n\n"},
		{name: "initial twice", stream: "event: initial\ndata: {}\n\nevent: initial\ndata: {}\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, test.stream)
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
			if test.name == "initial twice" {
				if initial := nextEvent(t, watcher); initial.Type != config.EventInitial {
					t.Fatalf("first event = %#v", initial)
				}
			}
			select {
			case event := <-watcher.Events():
				if event.Error == nil {
					t.Fatalf("terminal event = %#v, want protocol error", event)
				}
			case <-t.Context().Done():
				t.Fatal("timed out waiting for protocol error")
			}
		})
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
