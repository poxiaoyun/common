package command

import (
	"fmt"
	"strings"

	libreflect "xiaoshiai.cn/common/reflect"
)

// UnknownCommandError describes a failed command selection.
type UnknownCommandError struct {
	Command    string
	Usage      string
	Candidates []string
}

func (err *UnknownCommandError) Error() string {
	return fmt.Sprintf("unknown command %q for %q", err.Command, err.Usage)
}

type parsedCommand struct {
	command     Command
	path        []string
	selected    bool
	arguments   []string
	options     any
	schema      *libreflect.Node
	flags       []compiledFlag
	sourceFlags map[int][]FlagValue
	pluginFlags map[int][]FlagValue
}

func parseCommand(root Command, arguments []string, sources []Source, globalFlags []compiledGlobalFlag) (parsedCommand, error) {
	result := parsedCommand{
		command:     root,
		sourceFlags: map[int][]FlagValue{},
		pluginFlags: map[int][]FlagValue{},
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if result.command.Run != nil && argument == "--" {
			if err := selectAction(&result, sources, globalFlags); err != nil {
				return parsedCommand{}, err
			}
			result.arguments = append(result.arguments, arguments[index+1:]...)
			break
		}
		option, parsed := parseOption(argument)
		if option {
			if flag, exists := matchGlobalFlag(globalFlags, parsed); exists {
				value, consumed, err := parseOptionValue(parsed, flag.ValueMode, arguments[index:])
				if err != nil {
					return parsedCommand{}, err
				}
				recordGlobalFlag(&result, flag, newFlagValue(parsed, flag.compiledFlag, value, index))
				if flag.stopRouting {
					if err := selectAction(&result, sources, globalFlags); err != nil {
						return parsedCommand{}, err
					}
					return result, nil
				}
				index += consumed - 1
				continue
			}
			if result.command.Run == nil {
				return parsedCommand{}, fmt.Errorf("unknown option %q for command %q", argument, commandUsage(root.Name, result.path))
			}
			if err := selectAction(&result, sources, globalFlags); err != nil {
				return parsedCommand{}, err
			}
			flag, exists := matchActionFlag(result.flags, parsed)
			if !exists {
				return parsedCommand{}, fmt.Errorf("unknown option %q", argument)
			}
			value, consumed, err := parseOptionValue(parsed, flag.ValueMode, arguments[index:])
			if err != nil {
				return parsedCommand{}, err
			}
			result.sourceFlags[flag.sourceIndex] = append(result.sourceFlags[flag.sourceIndex], newFlagValue(parsed, *flag, value, index))
			index += consumed - 1
			continue
		}
		if !result.selected {
			if child := commandChild(result.command, argument); child != nil {
				result.command = *child
				result.path = append(result.path, argument)
				continue
			}
		}
		if result.command.Run == nil {
			return parsedCommand{}, unknownCommand(root.Name, result.command, result.path, argument)
		}
		if err := selectAction(&result, sources, globalFlags); err != nil {
			return parsedCommand{}, err
		}
		result.arguments = append(result.arguments, argument)
	}
	if result.command.Run != nil {
		if err := selectAction(&result, sources, globalFlags); err != nil {
			return parsedCommand{}, err
		}
		if err := validateArguments(result.command, result.arguments); err != nil {
			return parsedCommand{}, err
		}
	}
	return result, nil
}

func recordGlobalFlag(result *parsedCommand, flag *compiledGlobalFlag, value FlagValue) {
	switch flag.owner {
	case pluginGlobalFlag:
		result.pluginFlags[flag.ownerIndex] = append(result.pluginFlags[flag.ownerIndex], value)
	case sourceGlobalFlag:
		result.sourceFlags[flag.ownerIndex] = append(result.sourceFlags[flag.ownerIndex], value)
	}
}

func selectAction(result *parsedCommand, sources []Source, globalFlags []compiledGlobalFlag) error {
	if result.command.Run == nil || result.selected {
		return nil
	}
	result.selected = true
	if result.command.Options == nil {
		return nil
	}
	result.options = result.command.Options()
	schema, flags, err := compileAction(result.command, result.options, sources, globalFlags)
	if err != nil {
		return err
	}
	result.schema = schema
	result.flags = flags
	return nil
}

func commandChild(command Command, name string) *Command {
	for index := range command.Children {
		if command.Children[index].Name == name {
			return &command.Children[index]
		}
	}
	return nil
}

func unknownCommand(root string, command Command, path []string, name string) error {
	candidates := make([]string, len(command.Children))
	for index := range command.Children {
		candidates[index] = command.Children[index].Name
	}
	return &UnknownCommandError{
		Command:    name,
		Usage:      commandUsage(root, path),
		Candidates: candidates,
	}
}

type parsedOption struct {
	name     string
	value    string
	assigned bool
	short    bool
	display  string
}

func parseOption(argument string) (bool, parsedOption) {
	if strings.HasPrefix(argument, "--") && len(argument) > 2 {
		name, value, assigned := strings.Cut(strings.TrimPrefix(argument, "--"), "=")
		return true, parsedOption{name: name, value: value, assigned: assigned, display: "--" + name}
	}
	if strings.HasPrefix(argument, "-") && argument != "-" {
		name, value, assigned := strings.Cut(strings.TrimPrefix(argument, "-"), "=")
		return true, parsedOption{name: name, value: value, assigned: assigned, short: true, display: "-" + name}
	}
	return false, parsedOption{}
}

func matchGlobalFlag(flags []compiledGlobalFlag, option parsedOption) (*compiledGlobalFlag, bool) {
	for index := range flags {
		if flags[index].matchOption(option.name, option.short) {
			return &flags[index], true
		}
	}
	return nil, false
}

func matchActionFlag(flags []compiledFlag, option parsedOption) (*compiledFlag, bool) {
	for index := range flags {
		if flags[index].matchOption(option.name, option.short) {
			return &flags[index], true
		}
	}
	return nil, false
}

func (flag compiledFlag) matchOption(name string, short bool) bool {
	if short {
		return flag.Short != "" && flag.Short == name
	}
	return flag.match(name)
}

func parseOptionValue(option parsedOption, mode FlagValueMode, arguments []string) (string, int, error) {
	if option.assigned {
		return option.value, 1, nil
	}
	if mode == BooleanFlagValue {
		return "true", 1, nil
	}
	if len(arguments) < 2 {
		return "", 0, fmt.Errorf("option %s requires a value", option.display)
	}
	return arguments[1], 2, nil
}

func newFlagValue(option parsedOption, flag compiledFlag, value string, position int) FlagValue {
	name := option.name
	if option.short {
		name = flag.Pattern
	}
	return FlagValue{Name: name, Value: value, Position: position}
}

func commandUsage(root string, path []string) string {
	if len(path) == 0 {
		return root
	}
	return root + " " + strings.Join(path, " ")
}
