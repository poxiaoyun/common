package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"xiaoshiai.cn/common/command"
	"xiaoshiai.cn/common/log"
)

type generatedCommandPlugin struct {
	run func(command.Invocation) error
}

func (generatedCommandPlugin) Name() string {
	return "generated-command"
}

func (plugin generatedCommandPlugin) Commands() []command.Command {
	return []command.Command{{Name: "generated", Run: plugin.run}}
}

func TestCommandPluginContributesRootCommands(t *testing.T) {
	called := false
	program := command.Program{
		Command: command.Command{Name: "tool"},
		Plugins: []command.Plugin{generatedCommandPlugin{run: func(command.Invocation) error {
			called = true
			return nil
		}}},
		Sources: []command.Source{},
	}
	if err := command.Exec(context.Background(), program, command.Execution{Arguments: []string{"generated"}}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("injected command was not executed")
	}
}

func TestVersionIsADefaultFlagAndCommand(t *testing.T) {
	for _, arguments := range [][]string{{"--version"}, {"version"}} {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			output := &bytes.Buffer{}
			rootCalled := false
			program := command.Program{Command: command.Command{
				Name: "tool",
				Run: func(command.Invocation) error {
					rootCalled = true
					return nil
				},
			}}
			if err := command.Exec(context.Background(), program, command.Execution{
				Arguments: arguments,
				Streams:   command.Streams{Output: output},
			}); err != nil {
				t.Fatal(err)
			}
			if rootCalled {
				t.Fatal("root action ran instead of the version plugin")
			}
			got := k8sversion.Info{}
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("decode version: %v", err)
			}
			if got.GitVersion == "" || got.GoVersion == "" || got.Platform == "" {
				t.Fatalf("incomplete version: %#v", got)
			}
		})
	}
}

func TestVersionFlagPrecedesImplicitGroupHelp(t *testing.T) {
	output := &bytes.Buffer{}
	program := command.Program{Command: command.Command{
		Name:     "tool",
		Children: []command.Command{{Name: "run", Run: func(command.Invocation) error { return nil }}},
	}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--version"},
		Streams:   command.Streams{Output: output},
	}); err != nil {
		t.Fatal(err)
	}
	got := k8sversion.Info{}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode version instead of help: %v\n%s", err, output.String())
	}
}

func TestLoggingPluginProvidesGlobalVerbosityFlag(t *testing.T) {
	t.Cleanup(func() {
		if err := log.SetVerbosity(0); err != nil {
			t.Fatal(err)
		}
	})
	enabled := false
	loggerInjected := false
	program := command.Program{Command: command.Command{
		Name: "tool",
		Children: []command.Command{{
			Name: "run",
			Run: func(invocation command.Invocation) error {
				enabled = log.FromContext(invocation.Context).V(2).Enabled()
				_, err := logr.FromContext(invocation.Context)
				loggerInjected = err == nil
				return nil
			},
		}},
	}}
	if err := command.Exec(context.Background(), program, command.Execution{Arguments: []string{"-v=2", "run"}}); err != nil {
		t.Fatal(err)
	}
	if !enabled || !loggerInjected {
		t.Fatalf("logging enabled = %t, logger injected = %t", enabled, loggerInjected)
	}
}

func TestLoggingPluginPreservesContextLogger(t *testing.T) {
	var output strings.Builder
	logger := funcr.New(func(prefix, args string) {
		output.WriteString(prefix)
		output.WriteString(args)
	}, funcr.Options{})
	ctx := log.NewContext(context.Background(), logger)
	program := command.Program{Command: command.Command{
		Name: "tool",
		Run: func(invocation command.Invocation) error {
			log.FromContext(invocation.Context).Info("action logger")
			return nil
		},
	}}
	if err := command.Exec(ctx, program, command.Execution{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "action logger") {
		t.Fatalf("context logger was replaced: %s", output.String())
	}
}

func TestHelpIsADefaultPlugin(t *testing.T) {
	root := command.Command{Name: "tool", Summary: "Tool", Run: func(command.Invocation) error { return nil }}
	program := command.Program{Command: root}
	output := &bytes.Buffer{}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--help"},
		Streams:   command.Streams{Output: output},
	}); err != nil {
		t.Fatal(err)
	}
	if help := output.String(); !strings.Contains(help, "-h, --help") ||
		!strings.Contains(help, "-v, --v int") ||
		!strings.Contains(help, "--version") ||
		!strings.Contains(help, "version          Print version information") ||
		!strings.Contains(help, "--config-file string") {
		t.Fatalf("unexpected help:\n%s", help)
	}

	withoutPlugins := command.Program{Command: root, Plugins: []command.Plugin{}}
	if err := command.Exec(context.Background(), withoutPlugins, command.Execution{Arguments: []string{"--help"}}); err == nil {
		t.Fatal("--help was accepted without the Help Plugin")
	}
}

func TestCommandSuggestionsPluginUsesStructuredRoutingError(t *testing.T) {
	root := command.Command{
		Name: "tool",
		Children: []command.Command{{
			Name: "create",
			Run:  func(command.Invocation) error { return nil },
		}},
	}
	program := command.Program{Command: root}
	err := command.Exec(context.Background(), program, command.Execution{Arguments: []string{"craete"}})
	unknown := &command.UnknownCommandError{}
	if !errors.As(err, &unknown) || !strings.Contains(err.Error(), `did you mean "create"?`) {
		t.Fatalf("error = %v", err)
	}

	withoutPlugins := command.Program{Command: root, Plugins: []command.Plugin{}}
	err = command.Exec(context.Background(), withoutPlugins, command.Execution{Arguments: []string{"craete"}})
	if !errors.As(err, &unknown) || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %v", err)
	}
}
