// Package command defines and runs complete executable programs.
package command

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	libreflect "xiaoshiai.cn/common/reflect"
)

// UnknownPropertyPolicy controls unknown properties in structured configuration values.
type UnknownPropertyPolicy uint8

const (
	// WarnUnknownProperties logs and ignores unknown properties.
	WarnUnknownProperties UnknownPropertyPolicy = iota
	// RejectUnknownProperties rejects unknown properties.
	RejectUnknownProperties
)

// Command is one node in a caller-assembled command definition tree.
type Command struct {
	Name      string
	Summary   string
	Children  []Command
	Arguments []Argument
	// Options returns a fresh pointer to this command's startup options for
	// each execution. Nil means the command has no startup configuration.
	Options func() any
	Run     func(Invocation) error
}

// Streams contains the input and outputs used by one execution.
type Streams struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
}

// Invocation contains runtime values available to an action.
type Invocation struct {
	Context       context.Context
	Arguments     []string
	globalOptions any
	options       any
	Streams       Streams
}

// GlobalOptions returns the Program's resolved global startup options.
func GlobalOptions[T any](invocation Invocation) *T {
	return invocation.globalOptions.(*T)
}

// Options returns the selected command's resolved startup options.
func Options[T any](invocation Invocation) *T {
	return invocation.options.(*T)
}

// Execution contains explicit external inputs for one Program execution.
type Execution struct {
	Arguments   []string
	Environment map[string]string
	ReadFile    func(string) ([]byte, error)
	Streams     Streams
}

// Program specifies a complete executable program.
type Program struct {
	Command
	// GlobalOptions returns fresh startup options shared by every action for
	// each execution. Nil means the Program has no global configuration.
	GlobalOptions func() any
	// Plugins extends execution behavior. Nil selects DefaultPlugins; an empty
	// slice disables plugins.
	Plugins []Plugin
	// Sources are applied from low to high priority. Nil selects DefaultSources;
	// an empty slice disables external configuration.
	Sources []Source
	// UnknownProperties controls unknown fields from structured Sources.
	UnknownProperties UnknownPropertyPolicy
}

func validateCommand(command Command) error {
	if err := validateArgumentDeclarations(command); err != nil {
		return err
	}
	names := map[string]struct{}{}
	for _, child := range command.Children {
		if _, exists := names[child.Name]; exists {
			return fmt.Errorf("command %q has duplicate subcommand %q", command.Name, child.Name)
		}
		names[child.Name] = struct{}{}
		if err := validateCommand(child); err != nil {
			return err
		}
	}
	return nil
}

func compileAction(command Command, options any, sources []Source, globalFlags []compiledGlobalFlag) (*libreflect.Node, []compiledFlag, error) {
	schema, err := compileConfigurationSchema(options)
	if err != nil {
		return nil, nil, fmt.Errorf("command %q configuration: %w", command.Name, err)
	}
	flags, err := compileSourceFlags(sources, schema)
	if err != nil {
		return nil, nil, fmt.Errorf("command %q flags: %w", command.Name, err)
	}
	for _, flag := range flags {
		for _, global := range globalFlags {
			if flagsOverlap(flag, global.compiledFlag) {
				return nil, nil, fmt.Errorf("command %q flag --%s overlaps global flag --%s", command.Name, flag.Pattern, global.Pattern)
			}
		}
	}
	return schema, flags, nil
}

// Exec executes a Program specification without process-level side effects.
func Exec(ctx context.Context, program Program, execution Execution) error {
	plugins := program.Plugins
	if plugins == nil {
		plugins = DefaultPlugins()
	}
	sources := program.Sources
	if sources == nil {
		sources = DefaultSources()
	}
	root := program.Command
	root.Children = slices.Clone(root.Children)
	for _, registered := range plugins {
		if plugin, exists := registered.(CommandPlugin); exists {
			root.Children = append(root.Children, plugin.Commands()...)
		}
	}
	switch program.UnknownProperties {
	case WarnUnknownProperties, RejectUnknownProperties:
	default:
		return fmt.Errorf("invalid unknown-property policy %d", program.UnknownProperties)
	}
	if err := validateCommand(root); err != nil {
		return err
	}
	var globalOptions any
	var globalSchema *libreflect.Node
	var err error
	if program.GlobalOptions != nil {
		globalOptions = program.GlobalOptions()
		globalSchema, err = compileConfigurationSchema(globalOptions)
		if err != nil {
			return fmt.Errorf("global configuration: %w", err)
		}
	}
	globalFlags, err := compileGlobalFlags(plugins, sources, globalSchema)
	if err != nil {
		return err
	}
	parsed, err := parseCommand(root, execution.Arguments, sources, globalFlags)
	if err != nil {
		return handleCommandError(plugins, err)
	}
	rootRuntime := CommandRuntime{command: &root, globalFlags: globalFlagDeclarations(globalFlags)}
	if len(parsed.path) == 0 {
		rootRuntime.flags = parsed.flags
	}
	commandRuntime := CommandRuntime{command: &parsed.command, flags: parsed.flags, globalFlags: rootRuntime.globalFlags}
	for pluginIndex, registered := range plugins {
		plugin, exists := registered.(FlagPlugin)
		if !exists {
			continue
		}
		handled := false
		ctx, handled, err = plugin.Handle(ctx, PluginInvocation{
			Root:    rootRuntime,
			Command: commandRuntime,
			Path:    slices.Clone(parsed.path),
			Values:  slices.Clone(parsed.pluginFlags[pluginIndex]),
			Streams: execution.Streams,
		})
		if err != nil || handled {
			return err
		}
	}
	if parsed.command.Run == nil {
		return fmt.Errorf("command %q requires a subcommand", commandUsage(program.Name, parsed.path))
	}
	invocation := Invocation{
		Context:   ctx,
		Arguments: parsed.arguments,
		Streams:   execution.Streams,
	}
	if globalSchema != nil {
		if err := configureOptions(ctx, program, execution, sources, globalOptions, globalSchema, Target{
			Executable: program.Name,
			Global:     true,
		}, parsed.globalSourceFlags, parsed.controlSourceFlags); err != nil {
			return err
		}
		invocation.globalOptions = globalOptions
	}
	if parsed.schema != nil {
		if err := configureOptions(ctx, program, execution, sources, parsed.options, parsed.schema, Target{
			Executable:  program.Name,
			CommandPath: slices.Clone(parsed.path),
		}, parsed.sourceFlags, parsed.controlSourceFlags); err != nil {
			return err
		}
		invocation.options = parsed.options
	}
	return parsed.command.Run(invocation)
}

func configureOptions(
	ctx context.Context,
	program Program,
	execution Execution,
	sources []Source,
	options any,
	schema *libreflect.Node,
	target Target,
	targetFlags map[int][]FlagValue,
	controlFlags map[int][]FlagValue,
) error {
	if err := logDefaultConfiguration(ctx, options, schema, program.UnknownProperties); err != nil {
		return err
	}
	for sourceIndex, source := range sources {
		flags := slices.Clone(controlFlags[sourceIndex])
		flags = append(flags, targetFlags[sourceIndex]...)
		input := SourceInput{
			Target:        target,
			Arguments:     slices.Clone(execution.Arguments),
			Environment:   maps.Clone(execution.Environment),
			ReadFile:      execution.ReadFile,
			Flags:         flags,
			Configuration: schema,
		}
		values, err := source.Load(ctx, input)
		if err != nil {
			return fmt.Errorf("load configuration source %q: %w", source.Name(), err)
		}
		for _, value := range values {
			if err := applyConfigurationSourceValue(
				ctx,
				options,
				schema,
				source.Name(),
				value,
				program.UnknownProperties,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func handleCommandError(plugins []Plugin, err error) error {
	for _, plugin := range plugins {
		if handler, exists := plugin.(CommandErrorPlugin); exists {
			err = handler.HandleCommandError(err)
		}
	}
	return err
}

// Main runs a Program with OS inputs, signal cancellation, the default
// logger, error printing, and process exit status.
func Main(program Program) {
	ctx, stop := SignalContext(context.Background())
	defer stop()
	execution := Execution{
		Arguments:   os.Args[1:],
		Environment: processEnvironment(),
		ReadFile:    os.ReadFile,
		Streams: Streams{
			Input:       os.Stdin,
			Output:      os.Stdout,
			ErrorOutput: os.Stderr,
		},
	}
	if err := Exec(ctx, program, execution); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func processEnvironment() map[string]string {
	environment := map[string]string{}
	for _, item := range os.Environ() {
		name, value, _ := strings.Cut(item, "=")
		environment[name] = value
	}
	return environment
}
