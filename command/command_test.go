package command_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"xiaoshiai.cn/common/command"
	libreflect "xiaoshiai.cn/common/reflect"
)

func commandTestStreams(output io.Writer) command.Streams {
	return command.Streams{Input: strings.NewReader(""), Output: output, ErrorOutput: io.Discard}
}

func TestProgramRoutesAndInvokesAction(t *testing.T) {
	output := &bytes.Buffer{}
	var arguments []string
	type contextKey struct{}
	root := command.Command{
		Name:    "tool",
		Summary: "Administration tool",
		Children: []command.Command{{
			Name:      "apply",
			Summary:   "Apply resources",
			Arguments: []command.Argument{{Name: "resources", Optional: true, Variadic: true}},
			Run: func(invocation command.Invocation) error {
				arguments = invocation.Arguments
				if invocation.Context.Value(contextKey{}) != "context-value" {
					t.Fatal("invocation did not preserve context")
				}
				_, err := io.WriteString(invocation.Streams.Output, "applied\n")
				return err
			},
		}},
	}
	program := command.Program{Command: root}
	ctx := context.WithValue(context.Background(), contextKey{}, "context-value")
	err := command.Exec(ctx, program, command.Execution{
		Arguments: []string{"apply", "first", "--", "--literal"},
		Streams:   commandTestStreams(output),
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "applied\n" || !reflect.DeepEqual(arguments, []string{"first", "--literal"}) {
		t.Fatalf("output = %q, arguments = %#v", output.String(), arguments)
	}
}

func TestProgramReturnsRoutingAndActionErrors(t *testing.T) {
	want := errors.New("run failed")
	program := command.Program{Command: command.Command{Name: "run", Run: func(command.Invocation) error { return want }}}
	if err := command.Exec(context.Background(), program, command.Execution{Streams: commandTestStreams(io.Discard)}); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"extra"},
		Streams:   commandTestStreams(io.Discard),
	}); err == nil {
		t.Fatal("positional argument was accepted")
	}
}

func TestPositionalArgumentsAndSeparator(t *testing.T) {
	type options struct {
		Value string `config:"value"`
	}
	var gotArguments []string
	gotValue := ""
	program := command.Program{
		Command: command.Command{
			Name:      "run",
			Arguments: []command.Argument{{Name: "arguments", Optional: true, Variadic: true}},
			Options:   func() any { return &options{Value: "default"} },
			Run: func(invocation command.Invocation) error {
				gotArguments = slices.Clone(invocation.Arguments)
				gotValue = command.Options[options](invocation).Value
				return nil
			},
		},
		Sources: []command.Source{command.CommandLineArguments()},
	}
	tests := []struct {
		name          string
		input         []string
		wantArguments []string
		wantValue     string
	}{
		{name: "positionals", input: []string{"first", "second"}, wantArguments: []string{"first", "second"}, wantValue: "default"},
		{name: "flag before positional", input: []string{"--value=flag", "first"}, wantArguments: []string{"first"}, wantValue: "flag"},
		{name: "flag after positional", input: []string{"first", "--value=flag"}, wantArguments: []string{"first"}, wantValue: "flag"},
		{name: "separator makes flag literal", input: []string{"--", "--value=literal"}, wantArguments: []string{"--value=literal"}, wantValue: "default"},
		{name: "separator makes negative number literal", input: []string{"--", "-1"}, wantArguments: []string{"-1"}, wantValue: "default"},
		{
			name:          "mixed",
			input:         []string{"first", "--value=flag", "second", "--", "--literal", "-1"},
			wantArguments: []string{"first", "second", "--literal", "-1"},
			wantValue:     "flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := command.Exec(context.Background(), program, command.Execution{Arguments: tt.input}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotArguments, tt.wantArguments) || gotValue != tt.wantValue {
				t.Fatalf("arguments = %#v, value = %q; want %#v, %q", gotArguments, gotValue, tt.wantArguments, tt.wantValue)
			}
		})
	}
}

func TestPositionalArgumentDeclarations(t *testing.T) {
	tests := []struct {
		name          string
		declarations  []command.Argument
		input         []string
		wantArguments []string
		wantError     string
	}{
		{
			name:         "required argument is missing",
			declarations: []command.Argument{{Name: "source"}},
			wantError:    `command "copy" requires at least 1 arguments, got 0`,
		},
		{
			name:          "required argument",
			declarations:  []command.Argument{{Name: "source"}},
			input:         []string{"input.txt"},
			wantArguments: []string{"input.txt"},
		},
		{
			name:         "too many arguments",
			declarations: []command.Argument{{Name: "source"}},
			input:        []string{"input.txt", "output.txt"},
			wantError:    `command "copy" accepts at most 1 arguments, got 2`,
		},
		{
			name:         "optional argument is omitted",
			declarations: []command.Argument{{Name: "name", Optional: true}},
		},
		{
			name:          "required and optional arguments",
			declarations:  []command.Argument{{Name: "resource"}, {Name: "name", Optional: true}},
			input:         []string{"deployment", "api"},
			wantArguments: []string{"deployment", "api"},
		},
		{
			name:         "required variadic argument is missing",
			declarations: []command.Argument{{Name: "files", Variadic: true}},
			wantError:    `command "copy" requires at least 1 arguments, got 0`,
		},
		{
			name:          "required variadic arguments",
			declarations:  []command.Argument{{Name: "files", Variadic: true}},
			input:         []string{"first.txt", "second.txt"},
			wantArguments: []string{"first.txt", "second.txt"},
		},
		{
			name:          "optional variadic arguments",
			declarations:  []command.Argument{{Name: "arguments", Optional: true, Variadic: true}},
			input:         []string{"first", "second", "third"},
			wantArguments: []string{"first", "second", "third"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			program := command.Program{Command: command.Command{
				Name:      "copy",
				Arguments: tt.declarations,
				Run: func(invocation command.Invocation) error {
					got = slices.Clone(invocation.Arguments)
					return nil
				},
			}}
			err := command.Exec(context.Background(), program, command.Execution{Arguments: tt.input})
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.wantArguments) {
				t.Fatalf("arguments = %#v, want %#v", got, tt.wantArguments)
			}
		})
	}
}

type applyingOptions struct {
	Value string `config:"value"`
}

type applyingSource struct {
	name          string
	flags         []command.Flag
	configuration *libreflect.Node
	load          func(command.SourceInput) ([]command.SourceValue, error)
	loadCall      int
}

func (source *applyingSource) Name() string {
	return source.name
}

func (source *applyingSource) Load(_ context.Context, input command.SourceInput) ([]command.SourceValue, error) {
	source.loadCall++
	if source.load != nil {
		return source.load(input)
	}
	return nil, nil
}

func (source *applyingSource) Flags(configuration *libreflect.Node) ([]command.Flag, error) {
	source.configuration = configuration
	return source.flags, nil
}

func TestEverySourceReceivesTheSameCompleteExecution(t *testing.T) {
	var got *applyingOptions
	first := &applyingSource{
		name: "first",
		flags: []command.Flag{{
			Pattern:   "profile",
			ValueMode: command.RequiredFlagValue,
			ValueName: "string",
		}},
		load: func(input command.SourceInput) ([]command.SourceValue, error) {
			if !reflect.DeepEqual(input.Arguments, []string{"--profile", "production"}) {
				t.Fatalf("arguments = %#v", input.Arguments)
			}
			if len(input.Flags) != 1 || input.Flags[0].Name != "profile" || input.Flags[0].Value != "production" {
				t.Fatalf("flags = %#v", input.Flags)
			}
			input.Arguments[0] = "changed"
			input.Environment["PROFILE"] = "changed"
			return []command.SourceValue{{Name: "first", Value: applyingOptions{Value: "first"}}}, nil
		},
	}
	second := &applyingSource{
		name: "second",
		load: func(input command.SourceInput) ([]command.SourceValue, error) {
			if !reflect.DeepEqual(input.Arguments, []string{"--profile", "production"}) {
				t.Fatalf("arguments = %#v", input.Arguments)
			}
			if len(input.Flags) != 0 {
				t.Fatalf("second source flags = %#v", input.Flags)
			}
			if input.Environment["PROFILE"] != "production" {
				t.Fatalf("environment = %#v", input.Environment)
			}
			return []command.SourceValue{{Name: "second", Value: applyingOptions{Value: "second"}}}, nil
		},
	}
	root := command.Command{
		Name:    "serve",
		Summary: "Run the server",
		Options: func() any { return &applyingOptions{Value: "default"} },
		Run: func(invocation command.Invocation) error {
			got = command.Options[applyingOptions](invocation)
			return nil
		},
	}
	program := command.Program{Command: root, Sources: []command.Source{first, second}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments:   []string{"--profile", "production"},
		Environment: map[string]string{"PROFILE": "production"},
		Streams:     commandTestStreams(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}
	if first.configuration == nil || first.configuration != second.configuration {
		t.Fatal("flag sources did not receive the same configuration tree")
	}
	if got.Value != "second" {
		t.Fatalf("options = %#v", got)
	}
}

func TestHelpUsesFlagSourceWithoutApplyingSources(t *testing.T) {
	output := &bytes.Buffer{}
	source := &applyingSource{
		name: "profile",
		flags: []command.Flag{{
			Pattern:   "profile",
			Short:     "p",
			ValueMode: command.RequiredFlagValue,
			ValueName: "string",
			Summary:   "Configuration profile",
		}},
	}
	root := command.Command{
		Name:    "serve",
		Summary: "Run the server",
		Options: func() any { return &applyingOptions{} },
		Run:     func(command.Invocation) error { return nil },
	}
	program := command.Program{Command: root, Sources: []command.Source{source}}
	if err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{"--help"},
		Streams:   commandTestStreams(output),
	}); err != nil {
		t.Fatal(err)
	}
	if source.loadCall != 0 || !strings.Contains(output.String(), "-p, --profile string") {
		t.Fatalf("load calls = %d, help:\n%s", source.loadCall, output.String())
	}
}

func TestFlagSourceSupportsShortOptions(t *testing.T) {
	seen := command.FlagValue{}
	source := &applyingSource{
		name: "profile",
		flags: []command.Flag{{
			Pattern:   "profile",
			Short:     "p",
			ValueMode: command.RequiredFlagValue,
		}},
		load: func(input command.SourceInput) ([]command.SourceValue, error) {
			seen = input.Flags[0]
			return nil, nil
		},
	}
	root := command.Command{
		Name:    "serve",
		Options: func() any { return &applyingOptions{} },
		Run:     func(command.Invocation) error { return nil },
	}
	program := command.Program{Command: root, Sources: []command.Source{source}}
	if err := command.Exec(context.Background(), program, command.Execution{Arguments: []string{"-p", "production"}}); err != nil {
		t.Fatal(err)
	}
	if seen.Name != "profile" || seen.Value != "production" {
		t.Fatalf("flag = %#v", seen)
	}
}

func TestExecReturnsDefinitionErrors(t *testing.T) {
	tests := []struct {
		name string
		root command.Command
	}{
		{
			name: "duplicate child",
			root: command.Command{
				Name: "tool",
				Children: []command.Command{
					{Name: "run", Run: func(command.Invocation) error { return nil }},
					{Name: "run", Run: func(command.Invocation) error { return nil }},
				},
			},
		},
		{
			name: "non-struct options",
			root: command.Command{
				Name:    "run",
				Options: func() any { return new(int) },
				Run:     func(command.Invocation) error { return nil },
			},
		},
		{
			name: "variadic argument before final argument",
			root: command.Command{
				Name:      "copy",
				Arguments: []command.Argument{{Name: "sources", Variadic: true}, {Name: "destination"}},
				Run:       func(command.Invocation) error { return nil },
			},
		},
		{
			name: "required argument after optional argument",
			root: command.Command{
				Name:      "get",
				Arguments: []command.Argument{{Name: "namespace", Optional: true}, {Name: "resource"}},
				Run:       func(command.Invocation) error { return nil },
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := command.Exec(context.Background(), command.Program{Command: tt.root}, command.Execution{}); err == nil {
				t.Fatal("invalid definition was accepted")
			}
		})
	}
}
