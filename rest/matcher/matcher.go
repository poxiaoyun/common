package matcher

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/exp/slices"
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
}

type Section []Element

func NoRegexpString(sections []Section) string {
	var b strings.Builder
	for _, section := range sections {
		for _, elem := range section {
			if elem.VarName != "" {
				b.WriteByte('{')
				b.WriteString(elem.VarName)
				b.WriteByte('}')
			} else {
				b.WriteString(elem.Pattern)
			}
			if elem.Greedy {
				b.WriteByte('*')
			}
		}
	}
	return b.String()
}

func (s Section) String() string {
	var b strings.Builder
	for _, elem := range s {
		b.WriteString(elem.Pattern)
		if elem.Greedy {
			b.WriteByte('*')
		}
	}
	return b.String()
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
						re, err := regexp.Compile("^" + regstr + "$")
						if err != nil {
							return nil, CompileError{Pattern: pattern, Position: pre + 1 + idx + 1, Str: regstr, Message: err.Error()}
						}
						elem.Validate = re
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

// ========== 优化后的评分机制 ==========

// SectionScore 优化后的评分结构
type SectionScore struct {
	Specificity int  // 具体度：常量字符数量
	HasRegex    int  // 有正则验证的变量数
	VarCount    int  // 变量数量
	HasGreedy   bool // 是否有贪婪匹配
	IsRootOnly  bool // 是否仅为根路径 "/"
}

// detailedScore 计算 Section 的详细评分
func (s Section) detailedScore() SectionScore {
	score := SectionScore{}

	// 特殊处理：单独的 "/" 路径
	if len(s) == 1 && s[0].Pattern == "/" && s[0].VarName == "" {
		score.IsRootOnly = true
		return score
	}

	for _, v := range s {
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
			if v.Greedy {
				score.HasGreedy = true
			}
		} else {
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

	// 1. 根路径 "/" 优先级高于带变量的路径
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

	// 4. 变量越少越优先
	if aScore.VarCount != bScore.VarCount {
		return aScore.VarCount - bScore.VarCount
	}

	// 5. 非贪婪优先于贪婪
	if aScore.HasGreedy != bScore.HasGreedy {
		if aScore.HasGreedy {
			return 1
		}
		return -1
	}

	return 0
}
