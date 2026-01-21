package validation

import (
	"fmt"
	"strings"
)

// FieldPath 表示字段路径，支持点分隔和 JSON Pointer 两种格式
type FieldPath struct {
	segments []PathSegment
}

// PathSegment 路径段
type PathSegment struct {
	Type  SegmentType
	Value string // 字段名或索引
}

// SegmentType 路径段类型
type SegmentType int

const (
	SegmentTypeField SegmentType = iota // 字段名
	SegmentTypeIndex                    // 数组索引
	SegmentTypeKey                      // Map 键
)

// NewFieldPath 创建空的字段路径
func NewFieldPath() *FieldPath {
	return &FieldPath{
		segments: make([]PathSegment, 0),
	}
}

// Clone 克隆路径
func (p *FieldPath) Clone() *FieldPath {
	newPath := &FieldPath{
		segments: make([]PathSegment, len(p.segments)),
	}
	copy(newPath.segments, p.segments)
	return newPath
}

// AppendField 追加字段名
func (p *FieldPath) AppendField(name string) *FieldPath {
	newPath := p.Clone()
	newPath.segments = append(newPath.segments, PathSegment{
		Type:  SegmentTypeField,
		Value: name,
	})
	return newPath
}

// AppendIndex 追加数组索引
func (p *FieldPath) AppendIndex(index int) *FieldPath {
	newPath := p.Clone()
	newPath.segments = append(newPath.segments, PathSegment{
		Type:  SegmentTypeIndex,
		Value: fmt.Sprintf("%d", index),
	})
	return newPath
}

// AppendKey 追加 Map 键
func (p *FieldPath) AppendKey(key string) *FieldPath {
	newPath := p.Clone()
	newPath.segments = append(newPath.segments, PathSegment{
		Type:  SegmentTypeKey,
		Value: key,
	})
	return newPath
}

// DotNotation 返回点分隔格式的路径
// 例如: spec.containers[0].name 或 metadata.labels[app]
func (p *FieldPath) DotNotation() string {
	if len(p.segments) == 0 {
		return ""
	}

	var parts []string
	for i, seg := range p.segments {
		switch seg.Type {
		case SegmentTypeField:
			if i == 0 {
				parts = append(parts, seg.Value)
			} else {
				parts = append(parts, "."+seg.Value)
			}
		case SegmentTypeIndex:
			parts = append(parts, fmt.Sprintf("[%s]", seg.Value))
		case SegmentTypeKey:
			parts = append(parts, fmt.Sprintf("[%s]", seg.Value))
		}
	}

	return strings.Join(parts, "")
}

// JSONPointer 返回 RFC 6901 JSON Pointer 格式的路径
// 例如: /spec/containers/0/name
func (p *FieldPath) JSONPointer() string {
	if len(p.segments) == 0 {
		return ""
	}

	var parts []string
	for _, seg := range p.segments {
		// RFC 6901 转义: ~ -> ~0, / -> ~1
		escaped := escapeJSONPointer(seg.Value)
		parts = append(parts, escaped)
	}

	return "/" + strings.Join(parts, "/")
}

// IsEmpty 检查路径是否为空
func (p *FieldPath) IsEmpty() bool {
	return len(p.segments) == 0
}

// escapeJSONPointer 对 JSON Pointer 中的特殊字符进行转义
func escapeJSONPointer(s string) string {
	// 按照 RFC 6901，需要先转义 ~，再转义 /
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

// unescapeJSONPointer 反转义 JSON Pointer
func unescapeJSONPointer(s string) string {
	// 按照 RFC 6901，需要先反转义 ~1，再反转义 ~0
	s = strings.ReplaceAll(s, "~1", "/")
	s = strings.ReplaceAll(s, "~0", "~")
	return s
}

// ParseDotNotation 从点分隔格式解析路径
func ParseDotNotation(path string) *FieldPath {
	if path == "" {
		return NewFieldPath()
	}

	fp := NewFieldPath()
	current := ""

	for i := 0; i < len(path); i++ {
		ch := path[i]

		switch ch {
		case '.':
			if current != "" {
				fp.segments = append(fp.segments, PathSegment{
					Type:  SegmentTypeField,
					Value: current,
				})
				current = ""
			}
		case '[':
			if current != "" {
				fp.segments = append(fp.segments, PathSegment{
					Type:  SegmentTypeField,
					Value: current,
				})
				current = ""
			}
			// 找到 ]
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if j < len(path) {
				indexOrKey := path[i+1 : j]
				// 判断是索引还是键
				segType := SegmentTypeKey
				if isNumeric(indexOrKey) {
					segType = SegmentTypeIndex
				}
				fp.segments = append(fp.segments, PathSegment{
					Type:  segType,
					Value: indexOrKey,
				})
				i = j
			}
		default:
			current += string(ch)
		}
	}

	if current != "" {
		fp.segments = append(fp.segments, PathSegment{
			Type:  SegmentTypeField,
			Value: current,
		})
	}

	return fp
}

// ParseJSONPointer 从 JSON Pointer 格式解析路径
func ParseJSONPointer(pointer string) *FieldPath {
	if pointer == "" || pointer == "/" {
		return NewFieldPath()
	}

	// 移除开头的 /
	pointer = strings.TrimPrefix(pointer, "/")

	fp := NewFieldPath()
	parts := strings.Split(pointer, "/")

	for _, part := range parts {
		if part == "" {
			continue
		}

		unescaped := unescapeJSONPointer(part)

		// 判断是索引还是字段
		segType := SegmentTypeField
		if isNumeric(unescaped) {
			segType = SegmentTypeIndex
		}

		fp.segments = append(fp.segments, PathSegment{
			Type:  segType,
			Value: unescaped,
		})
	}

	return fp
}

// isNumeric 检查字符串是否为数字
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// DotToJSONPointer 将点分隔路径转换为 JSON Pointer
func DotToJSONPointer(dotPath string) string {
	return ParseDotNotation(dotPath).JSONPointer()
}

// JSONPointerToDot 将 JSON Pointer 转换为点分隔路径
func JSONPointerToDot(jsonPointer string) string {
	return ParseJSONPointer(jsonPointer).DotNotation()
}
