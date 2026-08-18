package command_test

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"xiaoshiai.cn/common/command"
)

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
