package wildcard

import (
	"slices"
	"strings"
)

const (
	SectionSep = ":"  // section separator
	MultiSep   = ","  // match any of these sections
	Star       = "*"  // match this section
	DoubleStar = "**" // match this section and all following sections
)

// acting like: https://shiro.apache.org/permissions.html#WildcardPermissions
// but extended to support ** to match all following sections
func Match(expr string, test string) bool {
	return Parse(expr).Match(ParseTest(test))
}

func (w Wildcard) Match(tests Test) bool {
	exprlen := len(w)
	for i, testsec := range tests {
		if i >= exprlen {
			return false // test has more sections than expr
		}
		if slices.Contains(w[i], DoubleStar) {
			return true // expr has wildcardAll, so it matches all remaining sections
		}
		ok := slices.ContainsFunc(w[i], func(s string) bool {
			return s == Star || s == testsec
		})
		if !ok {
			return false // testsec is not in exprsec
		}
	}
	// test has fewer sections than expr
	for i := len(tests); i < exprlen; i++ {
		if slices.Contains(w[i], DoubleStar) {
			return true // test has fewer sections than expr, but expr has wildcardAll, so it matches all remaining sections
		}
		if !slices.Contains(w[i], Star) {
			return false // test has fewer sections than expr, but expr has no wildcard, so it must not match
		}
	}
	return true
}

type Wildcard [][]string

func Parse(expr string) Wildcard {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	sectionstrs := strings.Split(expr, SectionSep)
	sections := make([][]string, len(sectionstrs))
	for i, section := range sectionstrs {
		sections[i] = strings.Split(section, MultiSep)
	}
	return sections
}

type Test []string

func (t Test) String() string {
	return strings.Join(t, SectionSep)
}

func ParseTest(test string) Test {
	return strings.Split(test, SectionSep)
}
