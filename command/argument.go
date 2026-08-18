package command

import (
	"context"
	"fmt"
	"strings"
)

// Completion is one value proposed for the word at the cursor.
type Completion struct {
	Value   string
	Summary string
}

// CompletionContext contains the command input relevant to one positional
// argument completion.
type CompletionContext struct {
	// CommandPath contains the selected command names below the Program root.
	CommandPath []string
	// Words contains the raw command-line words before the cursor.
	Words []string
	// Arguments contains the completed positional values before the cursor.
	Arguments []string
	// Prefix is the incomplete positional value at the cursor.
	Prefix string
}

// Argument declares one positional argument accepted by an executable Command.
type Argument struct {
	Name     string
	Summary  string
	Optional bool
	Variadic bool
	// Complete proposes values for this argument. Nil means that the argument
	// has no command-defined completions.
	Complete func(context.Context, CompletionContext) ([]Completion, error)
}

func validateArgumentDeclarations(command Command) error {
	optional := false
	for index, argument := range command.Arguments {
		if argument.Variadic && index != len(command.Arguments)-1 {
			return fmt.Errorf("command %q variadic argument %q must be last", command.Name, argument.Name)
		}
		if !argument.Optional && optional {
			return fmt.Errorf("command %q required argument %q cannot follow an optional argument", command.Name, argument.Name)
		}
		optional = optional || argument.Optional
	}
	return nil
}

func validateArguments(command Command, values []string) error {
	required := 0
	for _, argument := range command.Arguments {
		if !argument.Optional {
			required++
		}
	}
	if len(values) < required {
		return fmt.Errorf("command %q requires at least %d arguments, got %d", command.Name, required, len(values))
	}
	variadic := len(command.Arguments) != 0 && command.Arguments[len(command.Arguments)-1].Variadic
	if variadic || len(values) <= len(command.Arguments) {
		return nil
	}
	if len(command.Arguments) == 0 {
		return fmt.Errorf("command %q does not accept arguments: %s", command.Name, strings.Join(values, " "))
	}
	return fmt.Errorf("command %q accepts at most %d arguments, got %d", command.Name, len(command.Arguments), len(values))
}
