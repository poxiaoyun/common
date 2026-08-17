// Package pattern provides reusable string patterns.
package pattern

import (
	"fmt"
	"strings"
)

const (
	WildcardStar       = "*"
	WildcardDoubleStar = "**"
)

// WildcardOptions defines wildcard syntax.
type WildcardOptions struct {
	// Separator separates sections, for example ':' in "zoo:cats:*".
	Separator byte
}

// Wildcard matches separator-delimited strings. "*" matches within one
// non-empty section and "**" matches zero or more remaining non-empty sections.
// Commas separate candidates within a section; braces around candidates are
// optional, so "get,list" and "{get,list}" are equivalent.
type Wildcard struct {
	expression string
	separator  byte
}

// CompileWildcard returns a reusable matcher. Its successful path does not
// allocate.
func CompileWildcard(expression string, options WildcardOptions) (Wildcard, error) {
	if options.Separator == 0 || options.Separator == ',' {
		return Wildcard{}, fmt.Errorf("invalid wildcard separator")
	}
	return Wildcard{expression, options.Separator}, nil
}

// Match reports whether value matches without allocating.
func (p Wildcard) Match(value string) bool {
	expressionStart, valueStart := 0, 0
	for {
		expressionEnd := sectionEnd(p.expression, expressionStart, p.separator)
		valueEnd := sectionEnd(value, valueStart, p.separator)
		matched, remaining := matchSection(p.expression[expressionStart:expressionEnd], value[valueStart:valueEnd])
		if remaining {
			return remainingSectionsAreNonEmpty(value, valueStart, p.separator)
		}
		if !matched {
			return false
		}

		expressionDone := expressionEnd == len(p.expression)
		valueDone := valueEnd == len(value)
		if expressionDone || valueDone {
			if expressionDone {
				return valueDone
			}
			nextEnd := sectionEnd(p.expression, expressionEnd+1, p.separator)
			_, remaining = matchSection(p.expression[expressionEnd+1:nextEnd], "")
			return remaining
		}
		expressionStart, valueStart = expressionEnd+1, valueEnd+1
	}
}

func sectionEnd(value string, start int, separator byte) int {
	if end := strings.IndexByte(value[start:], separator); end >= 0 {
		return start + end
	}
	return len(value)
}

func matchSection(expression, value string) (matched, remaining bool) {
	if len(expression) >= 2 && expression[0] == '{' && expression[len(expression)-1] == '}' {
		expression = expression[1 : len(expression)-1]
	}
	for {
		end := strings.IndexByte(expression, ',')
		if end < 0 {
			end = len(expression)
		}
		candidate := expression[:end]
		if candidate == WildcardDoubleStar {
			return true, true
		}
		matched = matched || matchWildcardSection(candidate, value)
		if end == len(expression) {
			return matched, false
		}
		expression = expression[end+1:]
	}
}

func matchWildcardSection(expression, value string) bool {
	if value == "" && strings.IndexByte(expression, '*') >= 0 {
		return value != ""
	}
	expressionIndex, valueIndex, star, retry := 0, 0, -1, 0
	for valueIndex < len(value) {
		switch {
		case expressionIndex < len(expression) && expression[expressionIndex] == value[valueIndex]:
			expressionIndex, valueIndex = expressionIndex+1, valueIndex+1
		case expressionIndex < len(expression) && expression[expressionIndex] == '*':
			star, retry, expressionIndex = expressionIndex, valueIndex, expressionIndex+1
		case star >= 0:
			retry++
			expressionIndex, valueIndex = star+1, retry
		default:
			return false
		}
	}
	for expressionIndex < len(expression) && expression[expressionIndex] == '*' {
		expressionIndex++
	}
	return expressionIndex == len(expression)
}

func remainingSectionsAreNonEmpty(value string, start int, separator byte) bool {
	if start > len(value) || value == "" {
		return true
	}
	for i := start; i <= len(value); i++ {
		if i < len(value) && value[i] != separator {
			continue
		}
		if start == i {
			return false
		}
		start = i + 1
	}
	return true
}
