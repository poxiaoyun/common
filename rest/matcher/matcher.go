package matcher

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/exp/slices"
)

func ParseToken(path string) []string {
	tokens := []string{}
	pos := 0
	for i, char := range path {
		if char == '/' {
			if pos != i {
				tokens = append(tokens, path[pos:i])
			}
			pos = i
		}
	}
	if pos != len(path) {
		tokens = append(tokens, path[pos:])
	}
	return tokens
}

type Node[T any] struct {
	Section Section
	Value   T

	Children []*Node[T]
}

func (n *Node[T]) Get(pattern string) ([]Section, *Node[T], error) {
	sections, err := compileSections(pattern)
	if err != nil {
		return nil, nil, err
	}
	if len(sections) == 0 {
		return nil, nil, fmt.Errorf("empty pattern")
	}
	nodeapath := []*Node[T]{}
	cur := n
	for _, section := range sections {
		child := indexnode(cur, section)
		if child == nil {
			child = &Node[T]{Section: section}
			cur.Children = append(cur.Children, child)
			// sort children by score, so that we can match the most likely child first
			slices.SortFunc(cur.Children, func(a, b *Node[T]) int {
				ascore, bscore := a.Section.score(), b.Section.score()
				if ascore > bscore {
					return -1
				}
				if ascore < bscore {
					return 1
				}
				return 0
			})
		}
		nodeapath = append(nodeapath, child)
		cur = child
	}
	return sections, cur, nil
}

func indexnode[T any](node *Node[T], section Section) *Node[T] {
	for index, exists := range node.Children {
		if exists.Section.String() == section.String() {
			return node.Children[index]
		}
	}
	return nil
}

type Element struct {
	Pattern  string
	VarName  string
	Greedy   bool
	Validate *regexp.Regexp
}

type Section []Element

func NoRegexpString(sections []Section) string {
	str := ""
	for _, section := range sections {
		for _, elem := range section {
			if elem.VarName != "" {
				str += fmt.Sprintf("{%s}", elem.VarName)
			} else {
				str += elem.Pattern
			}
			if elem.Greedy {
				str += "*"
			}
		}
	}
	return str
}

func (s Section) String() string {
	patten := ""
	for _, elem := range s {
		patten += elem.Pattern
		if elem.Greedy {
			patten += "*"
		}
	}
	return patten
}

func (s Section) score() int {
	score := 0
	hasconst := false
	for _, v := range s {
		if v.Pattern == "/" {
			continue
		}
		if v.VarName != "" {
			score -= 10
			if v.Greedy {
				score -= 1
			}
			continue
		}
		if !hasconst {
			score += 100
			hasconst = true
		}
	}
	return score
}

func (n *Node[T]) Match(path string, oncandidate func(val T, vars []MatchVar) bool) (*Node[T], []MatchVar) {
	return n.match(ParseToken(path), nil, oncandidate)
}

func (n *Node[T]) match(tokens []string, vars []MatchVar, oncandidate func(val T, vars []MatchVar) bool) (*Node[T], []MatchVar) {
	for _, child := range n.Children {
		if ok, lefttokens, thisvars := child.Section.match(tokens); ok {
			if len(lefttokens) == 0 && (oncandidate == nil || oncandidate(child.Value, vars)) {
				return child, append(vars, thisvars...)
			}
			node, childvars := child.match(lefttokens, append(vars, thisvars...), oncandidate)
			if node != nil {
				return node, childvars
			}
		}
	}
	return nil, nil
}

type MatchVar struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

func (section Section) match(tokens []string) (bool, []string, []MatchVar) {
	if len(section) == 0 {
		return true, tokens, nil
	}
	pre := Element{}
	if len(tokens) == 0 {
		return false, tokens, nil
	}
	token, lefttokens, vars := tokens[0], tokens[1:], []MatchVar{}
	for _, elem := range section {
		if elem.Greedy {
			token, lefttokens = strings.Join(append([]string{token}, lefttokens...), ""), []string{}
		}
		if elem.VarName == "" {
			// lastIndex or Index?
			index := strings.Index(token, elem.Pattern)
			if index == -1 {
				return false, nil, nil
			}
			// finish pre var match
			if pre.VarName != "" {
				varmatch := token[:index]
				if (varmatch == "" && pre.VarName != "") || (pre.Validate != nil && !pre.Validate.MatchString(varmatch)) {
					return false, nil, nil
				}
				vars = append(vars, MatchVar{Name: pre.VarName, Value: varmatch})
			}
			token = token[index+len(elem.Pattern):]
		}
		pre = elem
	}
	// unclosed const greedy
	if pre.VarName == "" && pre.Greedy {
		token = ""
	}
	// unclosed variable
	if pre.VarName != "" {
		// regexp check
		if pre.Validate != nil && !pre.Validate.MatchString(token) {
			return false, nil, nil
		}
		vars = append(vars, MatchVar{Name: pre.VarName, Value: token})
		token = ""
	}
	// still left some chars
	if token != "" {
		return false, nil, nil
	}
	return true, lefttokens, vars
}

type CompileError struct {
	Pattern  string
	Position int
	Str      string
	Message  string
}

func (e CompileError) Error() string {
	return fmt.Sprintf("invalid [%s] in [%s] at position %d: %s", e.Str, e.Pattern, e.Position, e.Message)
}

func compileSections(patten string) ([]Section, error) {
	elems, err := compile(patten)
	if err != nil {
		return nil, err
	}
	sections := []Section{}
	pre := 0
	for i, elem := range elems {
		if elem.VarName != "" && elem.Greedy {
			return append(sections, elems[pre:]), nil
		}
		if elem.VarName == "" && strings.HasPrefix(elem.Pattern, "/") {
			if i != pre {
				sections = append(sections, elems[pre:i])
			}
			pre = i
		}
	}
	if pre != len(elems) {
		sections = append(sections, elems[pre:])
	}
	return sections, nil
}

// compile reads a variable name and a regular expression from a string.
func compile(pattern string) (Section, error) {
	elems := []Element{}
	pre, curly := -1, 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++ // skip the next char
		case '{':
			// open a variable
			if curly == 0 {
				// close pre section
				if pre != -1 {
					elems = append(elems, Element{Pattern: pattern[pre:i]})
				}
				pre = i
			}
			curly++
		case '}':
			if curly == 1 {
				varname := pattern[pre+1 : i]
				if varname == "" {
					varname = "_"
				}
				// close a variable
				elem := Element{
					Pattern: pattern[pre : i+1],
					VarName: varname,
				}
				if idx := strings.IndexRune(elem.VarName, ':'); idx != -1 {
					name, regstr := elem.VarName[:idx], elem.VarName[idx+1:]
					elem.VarName = name
					if regstr != "" {
						regexp, err := regexp.Compile("^" + regstr + "$")
						if err != nil {
							return nil, CompileError{Pattern: pattern, Position: pre + 1 + idx + 1, Str: regstr, Message: err.Error()}
						}
						elem.Validate = regexp
					}
				}
				// check greedy
				if i < len(pattern)-1 && pattern[i+1] == '*' {
					elem.Greedy = true
					i++
				}
				elems = append(elems, elem)
				pre = -1
			}
			curly--
		case '/':
			if curly != 0 {
				continue
			}
			if pre != -1 {
				elems = append(elems, Element{Pattern: pattern[pre:i]})
			}
			pre = i
		case '*':
			if curly != 0 {
				continue
			}
			if pre != -1 {
				elems = append(elems, Element{Pattern: pattern[pre:i], Greedy: true})
				pre = -1
			}
		default:
			// start const section
			if curly == 0 && pre == -1 {
				pre = i
			}
		}
	}
	// close the last const section
	if curly != 0 {
		return nil, CompileError{Pattern: pattern, Position: len(pattern) - 1, Message: "unclosed variable"}
	}
	if pre != -1 {
		lastpattern := pattern[pre:]
		if lastpattern[len(lastpattern)-1] == '*' {
			elems = append(elems, Element{Pattern: lastpattern[:len(lastpattern)-1], Greedy: true})
		} else {
			elems = append(elems, Element{Pattern: pattern[pre:]})
		}
	}
	return elems, nil
}
