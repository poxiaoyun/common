package commandsource_test

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaoshiai.cn/common/command"
	"xiaoshiai.cn/common/config"
	"xiaoshiai.cn/common/config/commandsource"
	configstore "xiaoshiai.cn/common/config/store"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/store/inmemory"
)

func TestSourceSelectsGlobalAndCommandConfiguration(t *testing.T) {
	client := newDynamicConfig(t)
	_, err := client.Set(t.Context(), "iam", "server", map[string]any{
		"global": map[string]any{"endpoint": "https://iam.example"},
		"serve":  map[string]any{"listen": ":8080"},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	source := commandsource.New(client, "iam", "server")

	global, err := source.Load(t.Context(), command.SourceInput{Target: command.Target{Executable: "iam", Global: true}})
	if err != nil {
		t.Fatalf("Load(global) error = %v", err)
	}
	if len(global) != 1 || global[0].Name != "server" || global[0].Value.(map[string]any)["endpoint"] != "https://iam.example" {
		t.Fatalf("Load(global) = %#v", global)
	}

	action, err := source.Load(t.Context(), command.SourceInput{Target: command.Target{Executable: "iam", CommandPath: []string{"serve"}}})
	if err != nil {
		t.Fatalf("Load(action) error = %v", err)
	}
	if len(action) != 1 || action[0].Value.(map[string]any)["listen"] != ":8080" {
		t.Fatalf("Load(action) = %#v", action)
	}
}

func TestSourceKeepsRootActionSeparateFromGlobalOptions(t *testing.T) {
	client := newDynamicConfig(t)
	_, err := client.Set(t.Context(), "iam", "server", map[string]any{
		"global": map[string]any{"endpoint": "https://iam.example"},
		"listen": ":8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := commandsource.New(client, "iam", "server").Load(t.Context(), command.SourceInput{
		Target: command.Target{Executable: "iam"},
	})
	if err != nil || len(values) != 1 {
		t.Fatalf("Load() = %#v, error = %v", values, err)
	}
	value := values[0].Value.(map[string]any)
	if _, exists := value["global"]; exists || value["listen"] != ":8080" {
		t.Fatalf("root action value = %#v", value)
	}
}

func TestFromOptionsDefaultsAddressSchemeToHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/namespaces/iam/configurations/server" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"server","resourceVersion":1,"value":{"global":{"endpoint":"https://iam.example"}}}`)
	}))
	defer server.Close()

	source := commandsource.FromOptions(commandsource.NewDefaultOptions(), "iam", "server")
	values, err := source.Load(t.Context(), command.SourceInput{
		Target: command.Target{Executable: "iam", Global: true},
		Environment: map[string]string{
			"CONFIGCENTER_ADDRESS": strings.TrimPrefix(server.URL, "http://") + "/v1",
			"CONFIGCENTER_TOKEN":   "secret",
		},
	})
	if err != nil || len(values) != 1 || values[0].Value.(map[string]any)["endpoint"] != "https://iam.example" {
		t.Fatalf("Load() = %#v, error = %v", values, err)
	}
}

func TestFromOptionsRejectsUnsupportedScheme(t *testing.T) {
	source := commandsource.FromOptions(&commandsource.Options{Address: "etcd://config.example"}, "iam", "server")
	_, err := source.Load(t.Context(), command.SourceInput{})
	if err == nil || !strings.Contains(err.Error(), `unsupported configuration center address scheme "etcd"`) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestDefaultSourcesUseNoopUntilAddressIsSupplied(t *testing.T) {
	sources := commandsource.DefaultSources("iam", "server")
	if len(sources) != 4 || sources[0].Name() != "configuration-files" ||
		sources[1].Name() != "configcenter" || sources[2].Name() != "environment" ||
		sources[3].Name() != "command-line" {
		t.Fatalf("DefaultSources() = %#v", sources)
	}
	values, err := sources[1].Load(t.Context(), command.SourceInput{})
	if err != nil || len(values) != 0 {
		t.Fatalf("noop Load() = %#v, error = %v", values, err)
	}
}

func TestDefaultSourcesLoadConfigurationThroughControlFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"server","resourceVersion":1,"value":{"endpoint":"https://configured.example"}}`)
	}))
	defer server.Close()

	type options struct {
		Endpoint string `json:"endpoint"`
	}
	resolved := ""
	program := command.Program{
		Command: command.Command{
			Name:    "server",
			Options: func() any { return &options{} },
			Run: func(invocation command.Invocation) error {
				resolved = command.Options[options](invocation).Endpoint
				return nil
			},
		},
		Sources: commandsource.DefaultSources("iam", "server"),
	}
	err := command.Exec(t.Context(), program, command.Execution{
		Arguments: []string{
			"--configcenter-address", strings.TrimPrefix(server.URL, "http://"),
			"--configcenter-token", "secret",
		},
		Environment: map[string]string{},
		ReadFile: func(string) ([]byte, error) {
			return nil, fs.ErrNotExist
		},
	})
	if err != nil || resolved != "https://configured.example" {
		t.Fatalf("Exec() endpoint = %q, error = %v", resolved, err)
	}
}

func TestSourceMissingConfigurationContributesNoValue(t *testing.T) {
	source := commandsource.New(newDynamicConfig(t), "iam", "optional")

	values, err := source.Load(t.Context(), command.SourceInput{Target: command.Target{Executable: "iam", Global: true}})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("Load() = %#v", values)
	}
}

func TestSourceDistinguishesMissingInputFromExistingEmptyObject(t *testing.T) {
	client := newDynamicConfig(t)
	if _, err := client.Set(t.Context(), "iam", "server", map[string]any{
		"global": map[string]any{},
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	source := commandsource.New(client, "iam", "server")

	missing, err := source.Load(t.Context(), command.SourceInput{
		Target: command.Target{CommandPath: []string{"serve"}},
	})
	if err != nil || len(missing) != 0 {
		t.Fatalf("Load(missing section) = %#v, error = %v", missing, err)
	}
	existing, err := source.Load(t.Context(), command.SourceInput{
		Target: command.Target{Global: true},
	})
	if err != nil || len(existing) != 1 || len(existing[0].Value.(map[string]any)) != 0 {
		t.Fatalf("Load(existing empty object) = %#v, error = %v", existing, err)
	}
}

func newDynamicConfig(t *testing.T) config.DynamicConfig {
	t.Helper()
	schema := store.NewSchema()
	if err := configstore.AddToSchema(schema); err != nil {
		t.Fatalf("AddToSchema() error = %v", err)
	}
	storage, err := inmemory.New(schema)
	if err != nil {
		t.Fatalf("inmemory.New() error = %v", err)
	}
	return configstore.New(storage)
}
