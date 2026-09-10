package matcher

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

func ParseToken(path string) []string {
	if path == "" {
		return nil
	}

	// 预估 token 数量（路径中 '/' 的数量）
	count := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			count++
		}
	}

	tokens := make([]string, 0, count)
	pos := 0

	// 使用索引而不是 range（避免 rune 转换）
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
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
	Section  Section
	Pattern  string
	Value    T
	Children []*Node[T]
	childMap map[string]*Node[T] // 快速查找缓存
}

func (n *Node[T]) Register(pattern string) ([]Section, *Node[T], error) {
	sections, err := CompilePattern(pattern)
	if err != nil {
		return nil, nil, err
	}
	if len(sections) == 0 {
		return nil, nil, fmt.Errorf("empty pattern")
	}
	cur := n
	for _, section := range sections {
		child := indexnode(cur, section)
		if child == nil {
			child = &Node[T]{Section: section}
			// 添加到 children 和 map
			cur.Children = append(cur.Children, child)
			if cur.childMap == nil {
				cur.childMap = make(map[string]*Node[T])
			}
			cur.childMap[section.String()] = child

			// 使用优化后的评分机制排序
			slices.SortFunc(cur.Children, func(a, b *Node[T]) int {
				return compareSectionOptimized(a.Section, b.Section)
			})
		}
		cur = child
	}
	cur.Pattern = pattern
	return sections, cur, nil
}

func indexnode[T any](node *Node[T], section Section) *Node[T] {
	// 优先使用 map 查找
	if node.childMap != nil {
		key := section.String()
		if child, ok := node.childMap[key]; ok {
			return child
		}
	}
	return nil
}

type Element struct {
	Pattern  string
	VarName  string
	Greedy   bool
	Validate *regexp.Regexp
	literal  string
	end      bool
}

type Section []Element

// PathTemplate projects a compiled pattern to a documentation path. Named
// captures become {name}; constraints, multi-segment markers, and {$} are omitted.
func PathTemplate(sections []Section) string {
	var b strings.Builder
	for _, section := range sections {
		for _, elem := range section {
			if elem.end {
				continue
			}
			if elem.VarName != "" {
				b.WriteByte('{')
				b.WriteString(elem.VarName)
				b.WriteByte('}')
			} else {
				b.WriteString(elem.Pattern)
			}
		}
	}
	return b.String()
}

func (s Section) String() string {
	var b strings.Builder
	for _, elem := range s {
		b.WriteString(elem.Pattern)
		if elem.Greedy && elem.VarName == "" && elem.Pattern != "/" {
			b.WriteByte('*')
		}
	}
	return b.String()
}

// Match accepts an escaped URL path and considers registered paths in segment specificity order. Candidate callbacks
// receive complete captures and may reject a path to continue. Copy captures
// that must be retained after rejecting a candidate. Captures are URL-decoded once.
func (n *Node[T]) Match(path string, oncandidate func(val T, vars []MatchVar) bool) (*Node[T], []MatchVar) {
	tokens := ParseToken(path)
	for i, part := range tokens {
		value, err := canonicalEscapes(part)
		if err != nil {
			return nil, nil
		}
		tokens[i] = value
	}
	return n.match(tokens, nil, oncandidate)
}

// Canonical escaping compares decoded characters without turning an escaped
// slash into a path separator. Requests retain their original URL encoding.
func canonicalEscapes(value string) (string, error) {
	parts := strings.Split(value, "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return "", err
		}
		parts[i] = url.PathEscape(decoded)
	}
	return strings.Join(parts, "/"), nil
}

func (n *Node[T]) match(tokens []string, vars []MatchVar, oncandidate func(val T, vars []MatchVar) bool) (*Node[T], []MatchVar) {
	for _, child := range n.Children {
		if ok, lefttokens, thisvars := child.Section.match(tokens); ok {
			matchedVars := append(vars, thisvars...)
			if len(lefttokens) == 0 && child.Pattern != "" && (oncandidate == nil || oncandidate(child.Value, matchedVars)) {
				return child, matchedVars
			}
			node, childvars := child.match(lefttokens, matchedVars, oncandidate)
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
	for position, elem := range section {
		if elem.end {
			return token == "" && len(lefttokens) == 0, lefttokens, vars
		}
		if elem.Greedy {
			token, lefttokens = strings.Join(append([]string{token}, lefttokens...), ""), []string{}
		}
		if elem.VarName == "" {
			var index int
			if (position == len(section)-1 || position == len(section)-2 && section[len(section)-1].end) && !elem.Greedy && pre.VarName != "" {
				// A final literal is a suffix of the entire segment, even
				// when the captured value contains the same literal.
				if !strings.HasSuffix(token, elem.literal) {
					return false, nil, nil
				}
				index = len(token) - len(elem.literal)
			} else {
				index = -1
				for start := 0; start <= len(token)-len(elem.literal); {
					if strings.HasPrefix(token[start:], elem.literal) {
						index = start
						break
					}
					// Literal matching must not begin inside an escaped byte.
					if token[start] == '%' {
						start += 3
					} else {
						start++
					}
				}
			}
			if index == -1 {
				return false, nil, nil
			}
			// finish pre var match
			if pre.VarName != "" {
				varmatch, err := url.PathUnescape(token[:index])
				if err != nil {
					return false, nil, nil
				}
				if (varmatch == "" && !pre.Greedy) || (pre.Validate != nil && !pre.Validate.MatchString(varmatch)) {
					return false, nil, nil
				}
				vars = append(vars, MatchVar{Name: pre.VarName, Value: varmatch})
			}
			token = token[index+len(elem.literal):]
		}
		pre = elem
	}
	// unclosed const greedy
	if pre.VarName == "" && pre.Greedy {
		token = ""
	}
	// unclosed variable
	if pre.VarName != "" {
		if token == "" && !pre.Greedy {
			return false, nil, nil
		}
		value, err := url.PathUnescape(token)
		if err != nil {
			return false, nil, nil
		}
		if pre.Validate != nil && !pre.Validate.MatchString(value) {
			return false, nil, nil
		}
		vars = append(vars, MatchVar{Name: pre.VarName, Value: value})
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

func CompilePattern(pattern string) ([]Section, error) {
	elems, err := Compile(pattern)
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

// Compile reads a variable name and a regular expression from a string.
func Compile(pattern string) (Section, error) {
	elems := []Element{}
	names := map[string]bool{}
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
			if curly == 0 {
				return nil, CompileError{Pattern: pattern, Position: i, Message: "unexpected closing brace"}
			}
			if curly == 1 {
				varname := pattern[pre+1 : i]
				// close a variable
				elem := Element{
					Pattern: pattern[pre : i+1],
					VarName: varname,
				}
				if varname == "$" {
					if i != len(pattern)-1 || pre == 0 || pattern[pre-1] != '/' {
						return nil, CompileError{Pattern: pattern, Position: pre, Message: "{$} must be the final path segment"}
					}
					elem.VarName, elem.end = "", true
				} else {
					name, expression, constrained := strings.Cut(varname, ":")
					elem.VarName, elem.Greedy = strings.CutSuffix(name, "...")
					if !validVariableName(elem.VarName) || names[elem.VarName] {
						return nil, CompileError{Pattern: pattern, Position: pre + 1, Str: elem.VarName, Message: "variable name must be a unique identifier"}
					}
					names[elem.VarName] = true
					if constrained {
						re, err := regexp.Compile("^(?:" + expression + ")$")
						if err != nil {
							return nil, CompileError{Pattern: pattern, Position: pre + 1 + len(name) + 1, Str: expression, Message: err.Error()}
						}
						elem.Validate = re
					}
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
			} else {
				return nil, CompileError{Pattern: pattern, Position: i, Message: "literal wildcard requires a prefix"}
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
		elems = append(elems, Element{Pattern: pattern[pre:], Greedy: pattern[pre:] == "/"})
	}
	for i := range elems {
		if elems[i].VarName != "" || elems[i].end {
			continue
		}
		literal, err := canonicalEscapes(elems[i].Pattern)
		if err != nil {
			return nil, CompileError{Pattern: pattern, Str: elems[i].Pattern, Message: err.Error()}
		}
		elems[i].literal = literal
	}
	return elems, nil
}

func validVariableName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// ========== 优化后的评分机制 ==========

// SectionScore 优化后的评分结构
type SectionScore struct {
	Specificity int  // 具体度：常量字符数量
	HasRegex    int  // 有正则验证的变量数
	VarCount    int  // 变量数量
	HasGreedy   bool // 是否有贪婪匹配
	IsRootOnly  bool // 是否为当前节点的精确尾斜杠路径
}

// detailedScore 计算 Section 的详细评分
func (s Section) detailedScore() SectionScore {
	score := SectionScore{}

	// 精确尾斜杠路径优先于同一节点下的通配路径。
	if len(s) == 2 && s[0].Pattern == "/" && s[1].end {
		score.IsRootOnly = true
		return score
	}

	for _, v := range s {
		score.HasGreedy = score.HasGreedy || v.Greedy
		// 跳过路径分隔符
		if v.Pattern == "/" {
			continue
		}

		if v.VarName != "" {
			// 变量
			score.VarCount++
			if v.Validate != nil {
				score.HasRegex++
			}
		} else if !v.end {
			// 常量：按字符长度计算具体度
			score.Specificity += len(v.Pattern)
		}
	}

	return score
}

// compareSectionOptimized 比较两个 Section 的优先级
// 返回值：< 0 表示 a 优先级更高，> 0 表示 b 优先级更高，= 0 表示相同
func compareSectionOptimized(a, b Section) int {
	aScore := a.detailedScore()
	bScore := b.detailedScore()

	// 1. 精确尾斜杠路径优先。
	if aScore.IsRootOnly != bScore.IsRootOnly {
		if aScore.IsRootOnly {
			return -1
		}
		return 1
	}

	// 2. 具体度最重要：常量字符越多越优先
	if aScore.Specificity != bScore.Specificity {
		return bScore.Specificity - aScore.Specificity
	}

	// 3. 有正则验证的变量优先级更高
	if aScore.HasRegex != bScore.HasRegex {
		return bScore.HasRegex - aScore.HasRegex
	}

	// Non-greedy segments precede anonymous and named multi-segment fallbacks.
	if aScore.HasGreedy != bScore.HasGreedy {
		if aScore.HasGreedy {
			return 1
		}
		return -1
	}
	if aScore.VarCount != bScore.VarCount {
		return aScore.VarCount - bScore.VarCount
	}

	return 0
}
