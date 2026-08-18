package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"xiaoshiai.cn/common/log"
)

// DefaultPlugins returns the standard command behavior.
func DefaultPlugins() []Plugin {
	return []Plugin{Logging(), Version(), Help(), CommandSuggestions()}
}

// Logging returns the global logging flag Plugin.
func Logging() LoggingPlugin {
	return LoggingPlugin{}
}

// LoggingPlugin applies the process-wide klog verbosity level.
type LoggingPlugin struct{}

func (LoggingPlugin) Name() string {
	return "logging"
}

func (LoggingPlugin) Flags() []GlobalFlag {
	return []GlobalFlag{{Flag: Flag{
		Pattern:   "v",
		Short:     "v",
		ValueMode: RequiredFlagValue,
		ValueName: "int",
		Summary:   "Number for the log level verbosity",
	}}}
}

func (LoggingPlugin) Handle(ctx context.Context, invocation PluginInvocation) (context.Context, bool, error) {
	ctx = log.NewContext(ctx, log.FromContext(ctx))
	if len(invocation.Values) == 0 {
		return ctx, false, nil
	}
	level, err := strconv.Atoi(invocation.Values[len(invocation.Values)-1].Value)
	if err != nil {
		return ctx, false, fmt.Errorf("option -v: %w", err)
	}
	if err := log.SetVerbosity(level); err != nil {
		return ctx, false, fmt.Errorf("option -v: %w", err)
	}
	return ctx, false, nil
}

// Help returns the default Help Plugin.
func Help() HelpPlugin {
	return HelpPlugin{}
}

// HelpPlugin renders help for groups and explicit -h or --help requests.
type HelpPlugin struct{}

func (HelpPlugin) Name() string {
	return "help"
}

func (HelpPlugin) Flags() []GlobalFlag {
	return []GlobalFlag{{
		Flag: Flag{
			Pattern:   "help",
			Short:     "h",
			ValueMode: BooleanFlagValue,
			Summary:   "Show help",
		},
		StopRouting: true,
	}}
}

func (HelpPlugin) Handle(ctx context.Context, invocation PluginInvocation) (context.Context, bool, error) {
	if invocation.Command.IsAction() && len(invocation.Values) == 0 {
		return ctx, false, nil
	}
	return ctx, true, writeCommandHelp(invocation.Streams.Output, invocation.Root, invocation.Command, invocation.Path)
}

func writeCommandHelp(output io.Writer, root, command CommandRuntime, path []string) error {
	if output == nil {
		output = io.Discard
	}
	usage := commandUsage(root.Name(), path)
	if command.Summary() != "" {
		fmt.Fprintln(output, command.Summary())
		fmt.Fprintln(output)
	}
	children := command.Children()
	declarations := command.Arguments()
	arguments := ""
	if command.IsAction() {
		for _, argument := range declarations {
			name := argument.Name
			if argument.Variadic {
				name += "..."
			}
			if argument.Optional {
				arguments += " [" + name + "]"
			} else {
				arguments += " <" + name + ">"
			}
		}
	}
	fmt.Fprintln(output, "Usage:")
	if command.IsAction() {
		fmt.Fprintf(output, "  %s [flags]%s\n", usage, arguments)
	}
	if len(children) != 0 {
		fmt.Fprintf(output, "  %s <command>\n", usage)
	} else if !command.IsAction() {
		fmt.Fprintf(output, "  %s\n", usage)
	}
	if len(children) != 0 {
		fmt.Fprintln(output, "\nAvailable Commands:")
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			fmt.Fprintf(output, "  %-16s %s\n", child.Name(), child.Summary())
		}
	}
	if len(declarations) != 0 {
		fmt.Fprintln(output, "\nArguments:")
		for _, argument := range declarations {
			name := argument.Name
			if argument.Variadic {
				name += "..."
			}
			fmt.Fprintf(output, "  %-16s %s\n", name, argument.Summary)
		}
	}
	flags := command.Flags()
	if len(flags) != 0 {
		fmt.Fprintln(output, "\nFlags:")
		sort.Slice(flags, func(i, j int) bool { return flags[i].Pattern < flags[j].Pattern })
		for _, flag := range flags {
			if flag.Hidden {
				continue
			}
			prefix := "      "
			if flag.Short != "" {
				prefix = "  -" + flag.Short + ", "
			}
			line := prefix + "--" + flag.Pattern
			if flag.ValueName != "" {
				line += " " + flag.ValueName
			}
			if flag.Summary != "" {
				line += "   " + flag.Summary
			}
			if flag.ShowDefault {
				line += " (default " + flag.DefaultValue + ")"
			}
			fmt.Fprintln(output, line)
		}
	}
	return nil
}

// CommandSuggestions returns the unknown-command suggestion Plugin.
func CommandSuggestions() CommandSuggestionsPlugin {
	return CommandSuggestionsPlugin{}
}

// CommandSuggestionsPlugin adds the nearest command to UnknownCommandError.
type CommandSuggestionsPlugin struct{}

func (CommandSuggestionsPlugin) Name() string {
	return "command-suggestions"
}

func (CommandSuggestionsPlugin) HandleCommandError(err error) error {
	var unknown *UnknownCommandError
	if !errors.As(err, &unknown) || len(unknown.Candidates) == 0 {
		return err
	}
	suggestion := unknown.Candidates[0]
	distance := editDistance(unknown.Command, suggestion)
	for _, candidate := range unknown.Candidates[1:] {
		if candidateDistance := editDistance(unknown.Command, candidate); candidateDistance < distance {
			suggestion, distance = candidate, candidateDistance
		}
	}
	if distance > 2 && distance*2 > len([]rune(unknown.Command)) {
		return err
	}
	return fmt.Errorf("%w; did you mean %q?", err, suggestion)
}

func editDistance(left, right string) int {
	leftRunes, rightRunes := []rune(left), []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range leftRunes {
		current := make([]int, len(previous))
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range rightRunes {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[rightIndex+1] = min(current[rightIndex]+1, previous[rightIndex+1]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	return previous[len(previous)-1]
}
