package command_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"xiaoshiai.cn/common/command"
)

var (
	_ command.GlobalFlagSource = command.ConfigurationFilesSource{}
	_ command.Source           = command.EnvironmentVariablesSource{}
	_ command.FlagSource       = command.CommandLineArgumentsSource{}
)

func sourceTestStreams(output io.Writer) command.Streams {
	return command.Streams{Input: strings.NewReader(""), Output: output, ErrorOutput: io.Discard}
}

type valueOptions struct {
	Value string `config:"value"`
}

func TestDefaultSourcesResolveFilesEnvironmentAndArguments(t *testing.T) {
	value := ""
	root := command.Command{
		Name:    "tool",
		Options: func() any { return &valueOptions{Value: "default"} },
		Run: func(invocation command.Invocation) error {
			value = command.Options[valueOptions](invocation).Value
			return nil
		},
	}
	program := command.Program{Command: root}
	readFile := func(path string) ([]byte, error) {
		if path == "tool.yaml" {
			return []byte("value: file\n"), nil
		}
		return nil, fs.ErrNotExist
	}
	execution := command.Execution{
		Arguments:   []string{"--value=argument"},
		Environment: map[string]string{"VALUE": "environment"},
		ReadFile:    readFile,
		Streams:     sourceTestStreams(io.Discard),
	}
	if err := command.Exec(context.Background(), program, execution); err != nil {
		t.Fatal(err)
	}
	if value != "argument" {
		t.Fatalf("value = %q, want argument", value)
	}

	execution.Arguments = nil
	if err := command.Exec(context.Background(), program, execution); err != nil {
		t.Fatal(err)
	}
	if value != "environment" {
		t.Fatalf("value = %q, want environment", value)
	}

	execution.Environment = nil
	if err := command.Exec(context.Background(), program, execution); err != nil {
		t.Fatal(err)
	}
	if value != "file" {
		t.Fatalf("value = %q, want file", value)
	}
}

func TestConfigurationFileKeepsRootActionSeparateFromGlobalOptions(t *testing.T) {
	values, err := (command.ConfigurationFilesSource{}).Load(t.Context(), command.SourceInput{
		Target: command.Target{Executable: "tool"},
		ReadFile: func(path string) ([]byte, error) {
			if path == "tool.yaml" {
				return []byte("global:\n  server: https://example.test\nvalue: action\n"), nil
			}
			return nil, fs.ErrNotExist
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("Load() = %#v", values)
	}
	value := values[0].Value.(map[string]any)
	if _, exists := value["global"]; exists || value["value"] != "action" {
		t.Fatalf("root action value = %#v", value)
	}
}

type valueSource struct {
	name   string
	values []command.SourceValue
	err    error
	calls  *[]string
}

type targetSource struct {
	targets *[]command.Target
}

func (targetSource) Name() string {
	return "targets"
}

func (source targetSource) Load(_ context.Context, input command.SourceInput) ([]command.SourceValue, error) {
	*source.targets = append(*source.targets, input.Target)
	if input.Target.Global {
		return []command.SourceValue{{Name: "global", Value: map[string]any{"server": "https://iam.example"}}}, nil
	}
	return []command.SourceValue{{Name: "action", Value: map[string]any{"output": "json"}}}, nil
}

func TestSourceLoadsGlobalAndActionTargetsIndependently(t *testing.T) {
	targets := []command.Target{}
	var gotGlobal *globalOptions
	var gotAction *listOptions
	program := command.Program{
		GlobalOptions: func() any { return &globalOptions{} },
		Command: command.Command{
			Name: "tool",
			Children: []command.Command{{
				Name:    "list",
				Options: func() any { return &listOptions{} },
				Run: func(invocation command.Invocation) error {
					gotGlobal = command.GlobalOptions[globalOptions](invocation)
					gotAction = command.Options[listOptions](invocation)
					return nil
				},
			}},
		},
		Sources: []command.Source{targetSource{targets: &targets}},
	}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"list"},
		Streams:   sourceTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || !targets[0].Global || targets[1].Global || !reflect.DeepEqual(targets[1].CommandPath, []string{"list"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if gotGlobal.Server != "https://iam.example" || gotAction.Output != "json" {
		t.Fatalf("global = %#v, action = %#v", gotGlobal, gotAction)
	}
}

func (source valueSource) Name() string {
	return source.name
}

func (source valueSource) Load(_ context.Context, _ command.SourceInput) ([]command.SourceValue, error) {
	if source.calls != nil {
		*source.calls = append(*source.calls, source.name)
	}
	if source.err != nil {
		return nil, source.err
	}
	return source.values, nil
}

type mergedOptions struct {
	Nested *mergedNestedOptions `config:"nested"`
	Labels map[string]string    `config:"labels"`
	Items  []string             `config:"items"`
}

type mergedNestedOptions struct {
	Name        string `config:"name"`
	Description string `config:"description"`
}

type jsonConfiguration struct {
	Value   string `config:"value"`
	Decoder string `config:"-"`
}

func (value *jsonConfiguration) UnmarshalJSON(data []byte) error {
	value.Value = string(data)
	value.Decoder = "json"
	return nil
}

type jsonAndTextConfiguration struct {
	Decoder string
}

func (value *jsonAndTextConfiguration) UnmarshalJSON([]byte) error {
	value.Decoder = "json"
	return nil
}

func (value *jsonAndTextConfiguration) UnmarshalText([]byte) error {
	value.Decoder = "text"
	return nil
}

func TestCommandLineConfigurationValues(t *testing.T) {
	type service struct {
		Address string `config:"address"`
		Enabled bool   `config:"enabled"`
	}
	type item struct {
		Name    string `config:"name"`
		Enabled bool   `config:"enabled"`
	}
	type options struct {
		Value             string             `config:"value"`
		IssuerPatterns    []string           `json:"issuerPatterns,omitempty"`
		AllowInsecureHTTP bool               `json:"allowInsecureHTTP,omitempty"`
		Numbers           []int              `config:"numbers"`
		Codes             [2]string          `config:"codes"`
		Labels            map[string]string  `config:"labels"`
		Services          map[string]service `config:"services"`
		Items             []item             `config:"items"`
	}

	tests := []struct {
		name      string
		defaults  options
		arguments []string
		want      options
	}{
		{
			name:      "scalar",
			defaults:  options{Value: "default"},
			arguments: []string{"--value=configured"},
			want:      options{Value: "configured"},
		},
		{
			name:      "json omitempty does not hide options",
			arguments: []string{"--issuerpatterns=http://127.0.0.1:8081", "--allowinsecurehttp"},
			want: options{
				IssuerPatterns:    []string{"http://127.0.0.1:8081"},
				AllowInsecureHTTP: true,
			},
		},
		{
			name:      "slice",
			defaults:  options{Numbers: []int{9}},
			arguments: []string{"--numbers=1,2,3"},
			want:      options{Numbers: []int{1, 2, 3}},
		},
		{
			name:      "array",
			defaults:  options{Codes: [2]string{"default", "values"}},
			arguments: []string{"--codes=first,second"},
			want:      options{Codes: [2]string{"first", "second"}},
		},
		{
			name:      "map",
			defaults:  options{Labels: map[string]string{"default": "kept"}},
			arguments: []string{`--labels={"region":"west","tier":"api"}`},
			want:      options{Labels: map[string]string{"default": "kept", "region": "west", "tier": "api"}},
		},
		{
			name:      "object map",
			defaults:  options{Services: map[string]service{"default": {Address: "https://default.example"}}},
			arguments: []string{`--services={"api":{"address":"https://api.example","enabled":true}}`},
			want: options{Services: map[string]service{
				"default": {Address: "https://default.example"},
				"api":     {Address: "https://api.example", Enabled: true},
			}},
		},
		{
			name:      "object array",
			defaults:  options{Items: []item{{Name: "default"}}},
			arguments: []string{`--items=[{"name":"first","enabled":true},{"name":"second"}]`},
			want: options{Items: []item{
				{Name: "first", Enabled: true},
				{Name: "second"},
			}},
		},
		{
			name:      "repeated scalar uses last value",
			defaults:  options{Value: "default"},
			arguments: []string{"--value=first", "--value=second"},
			want:      options{Value: "second"},
		},
		{
			name:      "repeated array uses last value",
			defaults:  options{Numbers: []int{9}},
			arguments: []string{"--numbers=1,2", "--numbers=3,4"},
			want:      options{Numbers: []int{3, 4}},
		},
		{
			name:      "repeated map keys merge",
			defaults:  options{Labels: map[string]string{"default": "kept"}},
			arguments: []string{"--labels[first]=one", "--labels[second]=two"},
			want:      options{Labels: map[string]string{"default": "kept", "first": "one", "second": "two"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got *options
			root := command.Command{
				Name:    "run",
				Options: func() any { value := test.defaults; return &value },
				Run: func(invocation command.Invocation) error {
					got = command.Options[options](invocation)
					return nil
				},
			}
			program := command.Program{Command: root, Sources: []command.Source{command.CommandLineArguments()}}
			if err := command.Exec(context.Background(), program, command.Execution{
				Arguments: test.arguments,
				Streams:   sourceTestStreams(io.Discard),
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, &test.want) {
				t.Fatalf("options = %#v, want %#v", got, &test.want)
			}
		})
	}
}

func TestStandardTimeConfigurationValues(t *testing.T) {
	type options struct {
		Duration  time.Duration `config:"duration"`
		StartedAt time.Time     `config:"startedAt"`
	}
	want := options{
		Duration:  30 * time.Second,
		StartedAt: time.Date(2026, time.August, 18, 22, 20, 57, 0, time.UTC),
	}
	tests := []struct {
		name      string
		source    command.Source
		execution command.Execution
	}{
		{
			name:   "command line",
			source: command.CommandLineArguments(),
			execution: command.Execution{
				Arguments: []string{"--duration=30s", "--startedat=2026-08-18T22:20:57Z"},
			},
		},
		{
			name:   "environment",
			source: command.EnvironmentVariables(),
			execution: command.Execution{
				Environment: map[string]string{"DURATION": "30s", "STARTEDAT": "2026-08-18T22:20:57Z"},
			},
		},
		{
			name:   "configuration file",
			source: command.ConfigurationFiles(),
			execution: command.Execution{
				ReadFile: func(path string) ([]byte, error) {
					if path == "run.yaml" {
						return []byte("duration: 30s\nstartedAt: 2026-08-18T22:20:57Z\n"), nil
					}
					return nil, fs.ErrNotExist
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got *options
			program := command.Program{
				Command: command.Command{
					Name:    "run",
					Options: func() any { return &options{} },
					Run: func(invocation command.Invocation) error {
						got = command.Options[options](invocation)
						return nil
					},
				},
				Sources: []command.Source{test.source},
			}
			if err := command.Exec(context.Background(), program, test.execution); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, &want) {
				t.Fatalf("options = %#v, want %#v", got, &want)
			}
		})
	}
}

func TestStructuredSourcesMergeValuesInOrder(t *testing.T) {
	var got *mergedOptions
	calls := []string{}
	root := command.Command{
		Name: "apply",
		Options: func() any {
			return &mergedOptions{
				Nested: &mergedNestedOptions{Name: "default", Description: "default"},
				Labels: map[string]string{"default": "kept", "removed": "old"},
				Items:  []string{"default"},
			}
		},
		Run: func(invocation command.Invocation) error {
			got = command.Options[mergedOptions](invocation)
			return nil
		},
	}
	low := valueSource{name: "low", calls: &calls, values: []command.SourceValue{{
		Name: "low",
		Value: map[string]any{
			"nested": map[string]any{"name": "low"},
			"labels": map[string]any{"low": "value"},
			"items":  []any{"low"},
		},
	}}}
	high := valueSource{name: "high", calls: &calls, values: []command.SourceValue{{
		Name: "high",
		Value: map[string]any{
			"nested": map[string]any{"description": "high"},
			"labels": map[string]any{"removed": nil, "high": "value"},
			"items":  []any{"high"},
		},
	}}}
	program := command.Program{Command: root, Sources: []command.Source{low, high}}
	if err := command.Exec(context.Background(), program, command.Execution{Streams: sourceTestStreams(io.Discard)}); err != nil {
		t.Fatal(err)
	}
	want := &mergedOptions{
		Nested: &mergedNestedOptions{Name: "low", Description: "high"},
		Labels: map[string]string{"default": "kept", "low": "value", "high": "value"},
		Items:  []string{"high"},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(calls, []string{"low", "high"}) {
		t.Fatalf("options = %#v, calls = %#v", got, calls)
	}
}

func TestCommandLineSupportsStructuredValuesAndStringDecoders(t *testing.T) {
	type options struct {
		Custom jsonConfiguration        `config:"custom"`
		Dual   jsonAndTextConfiguration `config:"dual"`
	}
	var got *options
	root := command.Command{
		Name:    "run",
		Options: func() any { return &options{} },
		Run: func(invocation command.Invocation) error {
			got = command.Options[options](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{command.CommandLineArguments()}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{`--custom={"value":"encoded"}`, `--dual={"value":"encoded"}`},
		Streams:   sourceTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	if got.Custom.Decoder != "json" || got.Custom.Value != `{"value":"encoded"}` || got.Dual.Decoder != "text" {
		t.Fatalf("options = %#v", got)
	}

	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--custom-value=structured"},
		Streams:   sourceTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	if got.Custom.Decoder != "" || got.Custom.Value != "structured" {
		t.Fatalf("options = %#v", got)
	}
}

func TestStructuredValuesRejectFlatPropertyPaths(t *testing.T) {
	root := command.Command{Name: "apply", Options: func() any { return &mergedOptions{} }, Run: func(command.Invocation) error { return nil }}
	program := command.Program{Command: root, Sources: []command.Source{valueSource{
		name: "flat",
		values: []command.SourceValue{{
			Name:  "flat",
			Value: map[string]any{"nested.name": "invalid"},
		}},
	}}}
	err := command.Exec(context.Background(), program, command.Execution{Streams: sourceTestStreams(io.Discard)})
	if err == nil || !strings.Contains(err.Error(), "must be a field name") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceFailureStopsLaterSourcesAndAction(t *testing.T) {
	want := errors.New("source failed")
	calls := []string{}
	actionCalled := false
	root := command.Command{
		Name:    "run",
		Options: func() any { return &valueOptions{} },
		Run: func(command.Invocation) error {
			actionCalled = true
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{
		valueSource{name: "failed", err: want, calls: &calls},
		valueSource{name: "skipped", calls: &calls},
	}}
	err := command.Exec(context.Background(), program, command.Execution{Streams: sourceTestStreams(io.Discard)})
	if !errors.Is(err, want) || actionCalled || !reflect.DeepEqual(calls, []string{"failed"}) {
		t.Fatalf("error = %v, action = %v, calls = %#v", err, actionCalled, calls)
	}
}

func TestConfigurationFilesUseExecutableAndCommandPath(t *testing.T) {
	value := ""
	root := command.Command{
		Name: "tool",
		Children: []command.Command{{
			Name:    "apply",
			Options: func() any { return &valueOptions{} },
			Run: func(invocation command.Invocation) error {
				value = command.Options[valueOptions](invocation).Value
				return nil
			},
		}},
	}
	program := command.Program{Command: root, Sources: []command.Source{command.ConfigurationFiles()}}
	files := map[string]string{
		"config/tool.yaml": "apply:\n  value: config-yaml\n",
		"config/tool.json": `{"apply":{"value":"config-json"}}`,
		"tool.yaml":        "apply:\n  value: root-yaml\n",
		"tool.json":        `{"apply":{"value":"root-json"}}`,
	}
	reads := []string{}
	readFile := func(path string) ([]byte, error) {
		reads = append(reads, path)
		value, exists := files[path]
		if !exists {
			return nil, fs.ErrNotExist
		}
		return []byte(value), nil
	}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"apply"},
		ReadFile:  readFile,
		Streams:   sourceTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	wantReads := []string{"config/tool.yaml", "config/tool.json", "tool.yaml", "tool.json"}
	if value != "root-json" || !reflect.DeepEqual(reads, wantReads) {
		t.Fatalf("value = %q, reads = %#v", value, reads)
	}

	files["explicit.yaml"] = "apply:\n  value: explicit\n"
	reads = nil
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--config-file=explicit.yaml", "apply"},
		ReadFile:  readFile,
		Streams:   sourceTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	if value != "explicit" || !reflect.DeepEqual(reads, []string{"explicit.yaml"}) {
		t.Fatalf("value = %q, reads = %#v", value, reads)
	}
}

func TestMapPropertiesSupportEnvironmentAndPerKeyArguments(t *testing.T) {
	type service struct {
		Endpoint string `config:"endpoint"`
		Enabled  bool   `config:"enabled"`
	}
	type options struct {
		Labels   map[string]string  `config:"labels"`
		Services map[string]service `config:"services"`
	}
	var got *options
	root := command.Command{
		Name: "run",
		Options: func() any {
			return &options{Labels: map[string]string{"default": "kept"}, Services: map[string]service{}}
		},
		Run: func(invocation command.Invocation) error {
			got = command.Options[options](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{command.EnvironmentVariables(), command.CommandLineArguments()}}
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{
			"--labels[cli]=argument",
			"--services[worker]-endpoint=https://worker.example",
			"--services[worker]-enabled",
		},
		Environment: map[string]string{
			"LABELS_ITEM":           "environment",
			"SERVICES_API_ENDPOINT": "https://api.example",
			"SERVICES_API_ENABLED":  "true",
		},
		Streams: sourceTestStreams(io.Discard),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := &options{
		Labels: map[string]string{"default": "kept", "item": "environment", "cli": "argument"},
		Services: map[string]service{
			"api":    {Endpoint: "https://api.example", Enabled: true},
			"worker": {Endpoint: "https://worker.example", Enabled: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestUnknownPropertiesCanWarnOrFail(t *testing.T) {
	called := false
	root := command.Command{
		Name:    "run",
		Options: func() any { return &valueOptions{} },
		Run: func(command.Invocation) error {
			called = true
			return nil
		},
	}
	source := valueSource{name: "custom", values: []command.SourceValue{{
		Name:  "custom",
		Value: map[string]any{"unknown": "value"},
	}}}
	program := command.Program{Command: root, Sources: []command.Source{source}}
	if err := command.Exec(context.Background(), program, command.Execution{Streams: sourceTestStreams(io.Discard)}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("action was not called in warning mode")
	}

	called = false
	strict := command.Program{Command: root, Sources: []command.Source{source}, UnknownProperties: command.RejectUnknownProperties}
	if err := command.Exec(context.Background(), strict, command.Execution{Streams: sourceTestStreams(io.Discard)}); err == nil {
		t.Fatal("strict mode accepted unknown property")
	}
	if called {
		t.Fatal("action ran after configuration failure")
	}
}
