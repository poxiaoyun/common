package command_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"xiaoshiai.cn/common/command"
	"xiaoshiai.cn/common/log"
)

type clearingSource struct{}

func (clearingSource) Name() string {
	return "clear"
}

type configurationValueSource struct {
	value command.SourceValue
}

type configurationValuesSource struct {
	name   string
	values []command.SourceValue
}

func (configurationValueSource) Name() string {
	return "value"
}

func (source configurationValueSource) Load(_ context.Context, _ command.SourceInput) ([]command.SourceValue, error) {
	return []command.SourceValue{source.value}, nil
}

func (source configurationValuesSource) Name() string {
	return source.name
}

func (source configurationValuesSource) Load(_ context.Context, _ command.SourceInput) ([]command.SourceValue, error) {
	return source.values, nil
}

func TestSourceValueLoggingDoesNotExposeNestedSensitiveValues(t *testing.T) {
	type credentials struct {
		Token string `config:"token,sensitive"`
	}
	type options struct {
		Credentials credentials `config:"credentials"`
	}
	root := command.Command{Name: "run", Options: func() any { return &options{} }, Run: func(command.Invocation) error { return nil }}
	source := configurationValueSource{value: command.SourceValue{
		Name: "credentials",
		Value: map[string]any{
			"credentials": map[string]any{"token": "do-not-log-this"},
		},
	}}
	program := command.Program{Command: root, Sources: []command.Source{source}}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 2})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "do-not-log-this") {
		t.Fatalf("sensitive value was logged: %s", output.String())
	}
}

func TestConfigurationValuesLogEveryProvidedInput(t *testing.T) {
	type options struct {
		Value string `config:"value"`
	}
	root := command.Command{Name: "run", Options: func() any { return &options{Value: "default"} }, Run: func(command.Invocation) error { return nil }}
	low := configurationValuesSource{name: "low", values: []command.SourceValue{{
		Name:  "low.yaml",
		Value: map[string]any{"value": "first"},
	}}}
	high := configurationValuesSource{name: "high", values: []command.SourceValue{{
		Name:  "VALUE",
		Value: map[string]any{"value": "first"},
	}}}
	program := command.Program{Command: root, Sources: []command.Source{low, high}}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 1})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, value := range []string{"config", "from", "key", "val", "default", "value", "low", "first", "high"} {
		if !strings.Contains(text, value) {
			t.Fatalf("configuration log does not contain %q: %s", value, text)
		}
	}
	for _, field := range []string{"input", "property", "previous", "changed"} {
		if strings.Contains(text, field) {
			t.Fatalf("configuration log contains %q: %s", field, text)
		}
	}
	for _, sourceKey := range []string{"low.yaml", "VALUE"} {
		if !strings.Contains(text, sourceKey) {
			t.Fatalf("configuration log does not contain source key %q: %s", sourceKey, text)
		}
	}
	if count := strings.Count(text, `"config"`); count != 3 {
		t.Fatalf("configuration log contains %d applied values, want 3: %s", count, text)
	}
}

func TestConfigurationLogsEnvironmentAndCommandLineKeys(t *testing.T) {
	type mongodb struct {
		Address string `config:"address"`
	}
	type options struct {
		Listen  string  `config:"listen"`
		Mongodb mongodb `config:"mongodb"`
	}
	root := command.Command{
		Name:    "run",
		Options: func() any { return &options{} },
		Run:     func(command.Invocation) error { return nil },
	}
	program := command.Program{
		Command: root,
		Sources: []command.Source{command.EnvironmentVariables(), command.CommandLineArguments()},
	}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 1})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{
		Arguments:   []string{"--mongodb-address=mongodb:27017"},
		Environment: map[string]string{"LISTEN": ":8080"},
	}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, fields := range []string{
		`"from"="environment" "key"="LISTEN"`,
		`"from"="command-line" "key"="--mongodb-address"`,
	} {
		if !strings.Contains(text, fields) {
			t.Fatalf("configuration log does not contain %q: %s", fields, text)
		}
	}
}

func TestDefaultConfigurationLoggingRedactsSensitiveValues(t *testing.T) {
	type options struct {
		Visible   string        `config:"visible"`
		Secret    string        `config:"secret,sensitive"`
		Runtime   string        `config:"-"`
		Timeout   time.Duration `config:"timeout"`
		StartedAt time.Time     `config:"startedAt"`
	}
	startedAt := time.Date(2026, time.August, 18, 22, 20, 57, 0, time.UTC)
	root := command.Command{
		Name: "run",
		Options: func() any {
			return &options{Visible: "shown", Secret: "hidden", Runtime: "runtime", Timeout: 30 * time.Second, StartedAt: startedAt}
		},
		Run: func(command.Invocation) error { return nil },
	}
	program := command.Program{Command: root, Sources: []command.Source{}}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 1})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, value := range []string{"default", "visible", "shown", "secret", "<redacted>", "timeout", "30s", "startedAt"} {
		if !strings.Contains(text, value) {
			t.Fatalf("default configuration log does not contain %q: %s", value, text)
		}
	}
	for _, value := range []string{"hidden", "runtime", "30000000000"} {
		if strings.Contains(text, value) {
			t.Fatalf("default configuration log contains %q: %s", value, text)
		}
	}
	if fields := `"key"="startedAt" "val"="2026-08-18T22:20:57Z"`; !strings.Contains(text, fields) {
		t.Fatalf("default configuration log does not contain RFC3339 time %q: %s", fields, text)
	}
}

func TestDefaultConfigurationLoggingUsesRuntimeNodeNamesAndValues(t *testing.T) {
	type database struct {
		Address string `config:"address"`
	}
	type options struct {
		Database database          `config:"database"`
		Labels   map[string]string `config:"labels"`
	}
	calls := 0
	root := command.Command{
		Name: "run",
		Options: func() any {
			calls++
			return &options{
				Database: database{Address: fmt.Sprintf("runtime-%d", calls)},
				Labels:   map[string]string{"region": "west"},
			}
		},
		Run: func(command.Invocation) error { return nil },
	}
	program := command.Program{Command: root, Sources: []command.Source{}}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 1})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, value := range []string{"database.address", "runtime-1", "labels[region]", "west"} {
		if !strings.Contains(text, value) {
			t.Fatalf("default configuration log does not contain %q: %s", value, text)
		}
	}
	if calls != 1 {
		t.Fatalf("options called %d times", calls)
	}
}

func TestEnvironmentAndFlagsLogSemanticNodeNames(t *testing.T) {
	type mongodb struct {
		Address string
	}
	type options struct {
		Listen  string
		Mongodb mongodb
	}
	root := command.Command{Name: "run", Options: func() any { return &options{} }, Run: func(command.Invocation) error { return nil }}
	program := command.Program{
		Command: root,
		Sources: []command.Source{command.EnvironmentVariables(), command.CommandLineArguments()},
	}
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{Verbosity: 1})
	ctx := log.NewContext(context.Background(), logger)
	if err := command.Exec(ctx, program, command.Execution{
		Arguments:   []string{"--mongodb-address=mongodb:27017"},
		Environment: map[string]string{"LISTEN": ":8080"},
	}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, value := range []string{"environment", "Listen", "command-line", "Mongodb.Address"} {
		if !strings.Contains(text, value) {
			t.Fatalf("configuration log does not contain %q: %s", value, text)
		}
	}
	for _, sourceKey := range []string{`key="LISTEN"`, `key="--mongodb-address"`} {
		if strings.Contains(text, sourceKey) {
			t.Fatalf("configuration log contains source key %q: %s", sourceKey, text)
		}
	}
}

func (clearingSource) Load(_ context.Context, _ command.SourceInput) ([]command.SourceValue, error) {
	return []command.SourceValue{{
		Name: "clear",
		Value: map[string]any{
			"nested": nil,
			"labels": map[string]any{"removed": nil},
		},
	}}, nil
}

func TestSourceValueNullClearsPointerAndDeletesMapEntry(t *testing.T) {
	type nested struct {
		Value string `config:"value"`
	}
	type options struct {
		Nested *nested           `config:"nested"`
		Labels map[string]string `config:"labels"`
	}
	var got *options
	root := command.Command{
		Name: "run",
		Options: func() any {
			return &options{
				Nested: &nested{Value: "value"},
				Labels: map[string]string{"kept": "value", "removed": "value"},
			}
		},
		Run: func(invocation command.Invocation) error {
			got = command.Options[options](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{clearingSource{}}}
	if err := command.Exec(context.Background(), program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	want := &options{Labels: map[string]string{"kept": "value"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestEmptyObjectAllocatesConfiguredPointer(t *testing.T) {
	type nested struct {
		Value string `config:"value"`
	}
	type options struct {
		Nested *nested `config:"nested"`
	}
	var got *options
	root := command.Command{
		Name:    "run",
		Options: func() any { return &options{Nested: &nested{Value: "default"}} },
		Run: func(invocation command.Invocation) error {
			got = command.Options[options](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{configurationValuesSource{
		name: "values",
		values: []command.SourceValue{
			{Name: "clear", Value: map[string]any{"nested": nil}},
			{Name: "empty", Value: map[string]any{"nested": map[string]any{}}},
		},
	}}}
	if err := command.Exec(context.Background(), program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	if got.Nested == nil || got.Nested.Value != "" {
		t.Fatalf("options = %#v", got)
	}
}
