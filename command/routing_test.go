package command_test

import (
	"bytes"
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"xiaoshiai.cn/common/command"
)

func newRoutingTestProgram() command.Program {
	return command.Program{Command: command.Command{
		Name:    "tool",
		Summary: "Administration tool",
		Children: []command.Command{{
			Name:    "user",
			Summary: "Manage users",
			Children: []command.Command{{
				Name:    "create",
				Summary: "Create a user",
				Run:     func(command.Invocation) error { return nil },
			}},
		}},
	}}
}

func TestGroupSelectionDisplaysHelp(t *testing.T) {
	program := newRoutingTestProgram()
	tests := []struct {
		name      string
		arguments []string
		usage     string
		child     string
	}{
		{name: "root without arguments", usage: "tool <command>", child: "user"},
		{name: "root long help", arguments: []string{"--help"}, usage: "tool <command>", child: "user"},
		{name: "root short help", arguments: []string{"-h"}, usage: "tool <command>", child: "user"},
		{name: "nested without arguments", arguments: []string{"user"}, usage: "tool user <command>", child: "create"},
		{name: "nested help", arguments: []string{"user", "--help"}, usage: "tool user <command>", child: "create"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			if err := command.Exec(context.Background(), program, command.Execution{
				Arguments: tt.arguments,
				Streams:   command.Streams{Output: output},
			}); err != nil {
				t.Fatal(err)
			}
			help := output.String()
			if !strings.Contains(help, tt.usage) || !strings.Contains(help, tt.child) {
				t.Fatalf("unexpected help:\n%s", help)
			}
		})
	}
}

func TestEmptyCommandDisplaysHelp(t *testing.T) {
	program := command.Program{
		Command: command.Command{Name: "tool", Summary: "Reserved command"},
		Plugins: []command.Plugin{command.Help()},
	}
	output := &bytes.Buffer{}
	if err := command.Exec(context.Background(), program, command.Execution{
		Streams: command.Streams{Output: output},
	}); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "Usage:\n  tool\n") || strings.Contains(help, "<command>") || strings.Contains(help, "Available Commands") {
		t.Fatalf("unexpected help:\n%s", help)
	}
}

func TestExecutableCommandRoutesChildrenUntilTheActionIsSelected(t *testing.T) {
	type options struct {
		Value string `config:"value"`
	}
	tests := []struct {
		name          string
		input         []string
		wantCommand   string
		wantArguments []string
		wantValue     string
		wantFactories int
	}{
		{name: "no input selects action", wantCommand: "root", wantValue: "default", wantFactories: 1},
		{name: "child name selects child", input: []string{"child"}, wantCommand: "child"},
		{name: "global flag does not select action", input: []string{"--config-file=config.yaml", "child"}, wantCommand: "child"},
		{name: "positional selects action", input: []string{"input", "child"}, wantCommand: "root", wantArguments: []string{"input", "child"}, wantValue: "default", wantFactories: 1},
		{name: "action flag selects action", input: []string{"--value=configured", "child"}, wantCommand: "root", wantArguments: []string{"child"}, wantValue: "configured", wantFactories: 1},
		{name: "separator selects action", input: []string{"--", "child"}, wantCommand: "root", wantArguments: []string{"child"}, wantValue: "default", wantFactories: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCommand := ""
			var gotArguments []string
			gotValue := ""
			factories := 0
			program := command.Program{
				Command: command.Command{
					Name:      "tool",
					Arguments: []command.Argument{{Name: "arguments", Optional: true, Variadic: true}},
					Options: func() any {
						factories++
						return &options{Value: "default"}
					},
					Run: func(invocation command.Invocation) error {
						gotCommand = "root"
						gotArguments = invocation.Arguments
						gotValue = command.Options[options](invocation).Value
						return nil
					},
					Children: []command.Command{{
						Name: "child",
						Run: func(command.Invocation) error {
							gotCommand = "child"
							return nil
						},
					}},
				},
				Plugins: []command.Plugin{},
				Sources: []command.Source{command.ConfigurationFiles(), command.CommandLineArguments()},
			}
			if err := command.Exec(context.Background(), program, command.Execution{
				Arguments: tt.input,
				ReadFile:  func(string) ([]byte, error) { return nil, fs.ErrNotExist },
			}); err != nil {
				t.Fatal(err)
			}
			if gotCommand != tt.wantCommand || !slices.Equal(gotArguments, tt.wantArguments) || gotValue != tt.wantValue || factories != tt.wantFactories {
				t.Fatalf("command = %q, arguments = %#v, value = %q, factories = %d; want %q, %#v, %q, %d", gotCommand, gotArguments, gotValue, factories, tt.wantCommand, tt.wantArguments, tt.wantValue, tt.wantFactories)
			}
		})
	}
}

func TestActionHelpDescribesPositionalArguments(t *testing.T) {
	program := command.Program{Command: command.Command{
		Name:    "copy",
		Summary: "Copy files",
		Arguments: []command.Argument{
			{Name: "source", Summary: "Source file"},
			{Name: "destination", Summary: "Destination file", Optional: true},
		},
		Run: func(command.Invocation) error { return nil },
	}}
	output := &bytes.Buffer{}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--help"},
		Streams:   command.Streams{Output: output},
	}); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	for _, expected := range []string{
		"Usage:\n  copy [flags] <source> [destination]",
		"Arguments:\n",
		"source           Source file",
		"destination      Destination file",
	} {
		if !strings.Contains(help, expected) {
			t.Fatalf("help does not contain %q:\n%s", expected, help)
		}
	}
}

func TestGroupSelectionRejectsUnknownInput(t *testing.T) {
	program := newRoutingTestProgram()
	tests := []struct {
		name      string
		arguments []string
		message   string
	}{
		{name: "root command", arguments: []string{"missing"}, message: `unknown command "missing" for "tool"`},
		{name: "nested command", arguments: []string{"user", "missing"}, message: `unknown command "missing" for "tool user"`},
		{name: "root option", arguments: []string{"--unknown"}, message: `unknown option "--unknown" for command "tool"`},
		{name: "nested option", arguments: []string{"user", "--unknown"}, message: `unknown option "--unknown" for command "tool user"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := command.Exec(context.Background(), program, command.Execution{Arguments: tt.arguments})
			if err == nil || err.Error() != tt.message {
				t.Fatalf("error = %v, want %q", err, tt.message)
			}
		})
	}
}
