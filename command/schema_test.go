package command_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"xiaoshiai.cn/common/command"
)

type globalOptions struct {
	Server string `json:"server"`
	Token  string `json:"token" config:"token,sensitive"`
}

type listOptions struct {
	Output    string    `json:"output" config:"output,short=o"`
	ExpiresAt time.Time `json:"expiresAt" config:"expires-at"`
}

func TestProgramResolvesGlobalAndActionOptions(t *testing.T) {
	var gotGlobal *globalOptions
	var gotAction *listOptions
	program := command.Program{
		GlobalOptions: func() any { return &globalOptions{Server: "default"} },
		Command: command.Command{
			Name: "tool",
			Children: []command.Command{{
				Name: "users",
				Children: []command.Command{{
					Name:    "list",
					Options: func() any { return &listOptions{Output: "table"} },
					Run: func(invocation command.Invocation) error {
						gotGlobal = command.GlobalOptions[globalOptions](invocation)
						gotAction = command.Options[listOptions](invocation)
						return nil
					},
				}},
			}},
		},
	}
	configuration := []byte("global:\n  server: file\n  token: file-secret\nusers:\n  list:\n    output: yaml\n")
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments:   []string{"--config-file", "tool.yaml", "users", "list", "-o", "json", "--token", "cli-secret"},
		Environment: map[string]string{"SERVER": "environment"},
		ReadFile: func(path string) ([]byte, error) {
			if path == "tool.yaml" {
				return configuration, nil
			}
			return nil, fs.ErrNotExist
		},
		Streams: schemaTestStreams(io.Discard),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (&globalOptions{Server: "environment", Token: "cli-secret"}); !reflect.DeepEqual(gotGlobal, want) {
		t.Fatalf("global options = %#v, want %#v", gotGlobal, want)
	}
	if want := (&listOptions{Output: "json"}); !reflect.DeepEqual(gotAction, want) {
		t.Fatalf("action options = %#v, want %#v", gotAction, want)
	}
}

func TestHelpShowsGlobalAndActionConfigurationFlags(t *testing.T) {
	program := command.Program{
		GlobalOptions: func() any { return &globalOptions{Token: "global-secret"} },
		Command: command.Command{
			Name: "tool",
			Children: []command.Command{{
				Name:    "list",
				Options: func() any { return &listOptions{} },
				Run:     func(command.Invocation) error { return nil },
			}},
		},
		Sources: []command.Source{command.CommandLineArguments()},
	}
	for _, arguments := range [][]string{{"--help"}, {"list", "--help"}} {
		output := &bytes.Buffer{}
		if err := command.Exec(context.Background(), program, command.Execution{
			Arguments: arguments,
			Streams:   schemaTestStreams(output),
		}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "--server") {
			t.Fatalf("global flag missing from help:\n%s", output)
		}
		if strings.Contains(output.String(), "global-secret") {
			t.Fatalf("sensitive global default included in help:\n%s", output)
		}
		if len(arguments) > 1 && (!strings.Contains(output.String(), "-o, --output") || !strings.Contains(output.String(), "--expires-at RFC3339")) {
			t.Fatalf("action flags missing from help:\n%s", output)
		}
	}
}

func schemaTestStreams(output io.Writer) command.Streams {
	return command.Streams{Input: strings.NewReader(""), Output: output, ErrorOutput: io.Discard}
}

type resourceProviderOptions struct {
	Issuer   string `config:"issuer" description:"Issuer URL"`
	ClientID string `config:"clientID" description:"Client identifier"`
}

type inlineOptions struct {
	Mode string `config:"mode"`
}

type configuredOptions struct {
	ResourceProvider *resourceProviderOptions `config:"resourceProvider"`
	ConfigName       string                   `config:"configName"`
	CommandName      string                   `json:"jsonCommandName" config:"cli-name"`
	GoName           string                   `json:"ignoredName"`
	Inline           inlineOptions            `config:",inline"`
	Labels           map[string]string        `config:"labels"`
	Count            int                      `config:"count"`
	Token            string                   `json:"token,omitempty" config:"token,sensitive"`
	Output           io.Writer                `config:"-"`
}

func TestProgramResolvesConfiguredAction(t *testing.T) {
	var got *configuredOptions
	root := command.Command{
		Name:    "server",
		Summary: "Run the server",
		Options: func() any {
			return &configuredOptions{
				ResourceProvider: &resourceProviderOptions{},
				ConfigName:       "default",
				CommandName:      "default",
				Token:            "factory-secret",
			}
		},
		Run: func(invocation command.Invocation) error {
			got = command.Options[configuredOptions](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{command.EnvironmentVariables(), command.CommandLineArguments()}}
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{
			"--resourceprovider-clientid=client",
			"--configname=flag",
			"--cli-name=command",
			"--ignoredname=go",
			"--mode=active",
			`--labels={"region":"west"}`,
			"--token=cli-secret",
		},
		Environment: map[string]string{
			"RESOURCEPROVIDER_ISSUER": "https://issuer.example",
			"CONFIGNAME":              "environment",
		},
		Streams: schemaTestStreams(io.Discard),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := &configuredOptions{
		ResourceProvider: &resourceProviderOptions{Issuer: "https://issuer.example", ClientID: "client"},
		ConfigName:       "flag",
		CommandName:      "command",
		GoName:           "go",
		Inline:           inlineOptions{Mode: "active"},
		Labels:           map[string]string{"region": "west"},
		Token:            "cli-secret",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
}

func TestHelpHidesSensitiveAndZeroDefaults(t *testing.T) {
	output := &bytes.Buffer{}
	root := command.Command{
		Name:    "server",
		Summary: "Run the server",
		Options: func() any { return &configuredOptions{ConfigName: "default", Token: "factory-secret"} },
		Run: func(command.Invocation) error {
			t.Fatal("action ran for help")
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{command.CommandLineArguments()}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--help"},
		Streams:   schemaTestStreams(output),
	}); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "--token") || !strings.Contains(help, "--configname string (default default)") || strings.Contains(help, "factory-secret") || strings.Contains(help, "default 0") {
		t.Fatalf("unexpected help:\n%s", help)
	}
	if strings.Contains(help, "resourceprovider") {
		t.Fatalf("nil pointer configuration was included in help:\n%s", help)
	}
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--resourceprovider-clientid=client"},
		Streams:   schemaTestStreams(io.Discard),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("nil pointer configuration option error = %v", err)
	}
}

func TestHelpShowsDefaultsFromNonNilPointerConfiguration(t *testing.T) {
	output := &bytes.Buffer{}
	root := command.Command{
		Name: "server",
		Options: func() any {
			return &configuredOptions{ResourceProvider: &resourceProviderOptions{Issuer: "https://issuer.example"}}
		},
		Run: func(command.Invocation) error { return nil },
	}
	program := command.Program{Command: root, Sources: []command.Source{command.CommandLineArguments()}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--help"},
		Streams:   schemaTestStreams(output),
	}); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "--resourceprovider-issuer string") || !strings.Contains(help, "Issuer URL") || !strings.Contains(help, "(default https://issuer.example)") {
		t.Fatalf("non-nil pointer configuration defaults missing from help:\n%s", help)
	}
}

func TestExecRejectsInvalidSchemas(t *testing.T) {
	tests := []struct {
		name string
		root command.Command
	}{
		{
			name: "duplicate flag",
			root: command.Command{
				Name: "run",
				Options: func() any {
					return &struct {
						First  string `config:"same"`
						Second string `config:"same"`
					}{}
				},
				Run: func(command.Invocation) error { return nil },
			},
		},
		{
			name: "unsupported runtime field",
			root: command.Command{
				Name:    "run",
				Options: func() any { return &struct{ Output io.Writer }{} },
				Run:     func(command.Invocation) error { return nil },
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := command.Exec(context.Background(), command.Program{Command: tt.root}, command.Execution{}); err == nil {
				t.Fatal("invalid schema was accepted")
			}
		})
	}
}

func TestSensitiveBindingErrorDoesNotExposeValue(t *testing.T) {
	type options struct {
		SecretCount int `config:"secretCount,sensitive"`
	}
	root := command.Command{Name: "tool", Options: func() any { return &options{} }, Run: func(command.Invocation) error { return nil }}
	program := command.Program{Command: root, Sources: []command.Source{command.CommandLineArguments()}}
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--secretcount=do-not-print-this"},
		Streams:   schemaTestStreams(io.Discard),
	})
	if err == nil || strings.Contains(err.Error(), "do-not-print-this") {
		t.Fatalf("error = %v", err)
	}
}
