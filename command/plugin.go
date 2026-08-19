package command

import (
	"context"
	"fmt"
	"slices"

	libreflect "xiaoshiai.cn/common/reflect"
)

// Plugin extends Program execution without becoming a configuration Source.
type Plugin interface {
	Name() string
}

// CommandPlugin contributes commands beneath the Program root.
type CommandPlugin interface {
	Plugin
	Commands() []Command
}

// GlobalFlag is a Plugin-owned flag accepted at every command level.
type GlobalFlag struct {
	Flag
	// StopRouting selects the command reached before this flag.
	StopRouting bool
}

// FlagPlugin handles its global flags after command and argument parsing.
type FlagPlugin interface {
	Plugin
	Flags() []GlobalFlag
	Handle(ctx context.Context, invocation PluginInvocation) (context.Context, bool, error)
}

// CommandErrorPlugin transforms command parsing and routing errors.
type CommandErrorPlugin interface {
	Plugin
	HandleCommandError(error) error
}

// PluginInvocation contains the selected command and values owned by one Plugin.
type PluginInvocation struct {
	// Root is the program root.
	Root CommandRuntime
	// Command is the selected command.
	Command CommandRuntime
	// Path contains the selected command names below Root.
	Path []string
	// Values contains occurrences of flags owned by this Plugin.
	Values []FlagValue
	// Streams are the current execution streams.
	Streams Streams
}

// CommandRuntime is the read-only runtime state derived from one Command.
type CommandRuntime struct {
	command     *Command
	flags       []compiledFlag
	globalFlags []Flag
}

// Name returns the command name.
func (runtime CommandRuntime) Name() string {
	return runtime.command.Name
}

// Summary returns the command summary.
func (runtime CommandRuntime) Summary() string {
	return runtime.command.Summary
}

// IsAction reports whether the command is executable.
func (runtime CommandRuntime) IsAction() bool {
	return runtime.command.Run != nil
}

// Arguments returns the command's positional argument declarations.
func (runtime CommandRuntime) Arguments() []Argument {
	return slices.Clone(runtime.command.Arguments)
}

// Children returns the command's immediate children.
func (runtime CommandRuntime) Children() []CommandRuntime {
	children := make([]CommandRuntime, len(runtime.command.Children))
	for index := range runtime.command.Children {
		children[index] = CommandRuntime{command: &runtime.command.Children[index], globalFlags: runtime.globalFlags}
	}
	return children
}

// Flags returns global and action flags accepted by the command.
func (runtime CommandRuntime) Flags() []Flag {
	flags := slices.Clone(runtime.globalFlags)
	if runtime.command.Run != nil {
		for _, flag := range runtime.flags {
			flags = append(flags, flag.Flag)
		}
	}
	return flags
}

type compiledGlobalFlag struct {
	compiledFlag
	owner       globalFlagOwner
	ownerIndex  int
	stopRouting bool
}

type globalFlagOwner uint8

const (
	pluginGlobalFlag globalFlagOwner = iota
	sourceGlobalFlag
	configurationGlobalFlag
)

func compileGlobalFlags(plugins []Plugin, sources []Source, globalConfiguration *libreflect.Node) ([]compiledGlobalFlag, error) {
	flags := []compiledGlobalFlag{}
	appendFlag := func(flag compiledGlobalFlag) error {
		for _, existing := range flags {
			if flagsOverlap(existing.compiledFlag, flag.compiledFlag) {
				return fmt.Errorf("global flags --%s and --%s overlap", existing.Pattern, flag.Pattern)
			}
		}
		flags = append(flags, flag)
		return nil
	}
	for pluginIndex, plugin := range plugins {
		owner, exists := plugin.(FlagPlugin)
		if !exists {
			continue
		}
		for _, declaration := range owner.Flags() {
			flag, err := compileFlag(declaration.Flag)
			if err != nil {
				return nil, fmt.Errorf("plugin %q: %w", plugin.Name(), err)
			}
			global := compiledGlobalFlag{
				compiledFlag: flag,
				owner:        pluginGlobalFlag,
				ownerIndex:   pluginIndex,
				stopRouting:  declaration.StopRouting,
			}
			if err := appendFlag(global); err != nil {
				return nil, err
			}
		}
	}
	for sourceIndex, source := range sources {
		owner, exists := source.(GlobalFlagSource)
		if !exists {
			continue
		}
		for _, declaration := range owner.GlobalFlags() {
			flag, err := compileFlag(declaration)
			if err != nil {
				return nil, fmt.Errorf("source %q: %w", source.Name(), err)
			}
			if err := appendFlag(compiledGlobalFlag{
				compiledFlag: flag,
				owner:        sourceGlobalFlag,
				ownerIndex:   sourceIndex,
			}); err != nil {
				return nil, err
			}
		}
	}
	if globalConfiguration != nil {
		declared, err := compileSourceFlags(sources, globalConfiguration)
		if err != nil {
			return nil, fmt.Errorf("global configuration flags: %w", err)
		}
		for _, flag := range declared {
			if err := appendFlag(compiledGlobalFlag{
				compiledFlag: flag,
				owner:        configurationGlobalFlag,
				ownerIndex:   flag.sourceIndex,
			}); err != nil {
				return nil, err
			}
		}
	}
	return flags, nil
}

func globalFlagDeclarations(flags []compiledGlobalFlag) []Flag {
	declarations := make([]Flag, len(flags))
	for index := range flags {
		declarations[index] = flags[index].Flag
	}
	return declarations
}
