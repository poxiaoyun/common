package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
)

const (
	ValidationTagName = "validation"
)

// ValidationError 表示单个验证错误
type ValidationError struct {
	// Path JSON Pointer 路径 (RFC 6901, 如 "/spec/containers/0/name")
	Path string `json:"path"`

	// Rule 验证规则名称 (如 "required", "min", "regexp")
	Rule string `json:"rule"`

	// Params 规则参数 (如 min 的值 ["1"], range 的值 ["1", "100"])
	Params []string `json:"params,omitempty"`

	// Actual 实际值 (用于错误提示，可能被截断或脱敏)
	Actual any `json:"actual,omitempty"`
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	if len(e.Params) > 0 {
		return fmt.Sprintf("%s: %s %s", e.Path, e.Rule, strings.Join(e.Params, " "))
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Rule)
}

// MessageKey 返回 i18n 消息键 (约定为 validation.{rule})
func (e ValidationError) MessageKey() string {
	return "validation." + e.Rule
}

// NewValidationError 创建验证错误的辅助函数
func NewValidationError(rule string, params []string, actual any) *ValidationError {
	return &ValidationError{
		Rule:   rule,
		Params: params,
		Actual: actual,
	}
}

// ValidationErrors 验证错误集合
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// Add 添加验证错误
func (e *ValidationErrors) Add(err ValidationError) {
	e.Errors = append(e.Errors, err)
}

// Error 实现 error 接口
func (e *ValidationErrors) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return ""
	}

	var messages []string
	for _, err := range e.Errors {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// HasErrors 检查是否有错误
func (e *ValidationErrors) HasErrors() bool {
	return e != nil && len(e.Errors) > 0
}

// Validator 验证器，返回结构化错误
type Validator struct {
	rules map[string]RuleFunc
}

// NewValidator 创建新的验证器
func NewValidator() *Validator {
	rules := make(map[string]RuleFunc, len(DefaultRules))
	maps.Copy(rules, DefaultRules)
	return &Validator{rules: rules}
}

// RegisterRule 注册自定义规则
func (v *Validator) RegisterRule(name string, fn RuleFunc) {
	v.rules[name] = fn
}

// Validate 验证数据，返回结构化错误
func (v *Validator) Validate(ctx context.Context, data any) *ValidationErrors {
	rv := reflect.ValueOf(data)
	if rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	collector := &ValidationErrors{}
	path := NewFieldPath()

	v.validateValue(ctx, path, "", rv, collector)
	if len(collector.Errors) == 0 {
		return nil
	}
	return collector
}

// DecodeAndValidate 解析 JSON 并验证
func (v *Validator) DecodeAndValidate(ctx context.Context, data []byte, obj any) *ValidationErrors {
	errs := &ValidationErrors{}

	if err := json.Unmarshal(data, obj); err != nil {
		if e, ok := err.(*json.UnmarshalTypeError); ok {
			path := ParseDotNotation(e.Field)
			errs.Add(ValidationError{
				Path:   path.JSONPointer(),
				Rule:   "type",
				Params: []string{getSimpleTypeName(e.Type)},
				Actual: e.Value,
			})
			return errs
		}
		errs.Add(ValidationError{
			Path:   "",
			Rule:   "json",
			Actual: err.Error(),
		})
		return errs
	}

	return v.Validate(ctx, obj)
}

// validateValue 验证单个值
func (v *Validator) validateValue(ctx context.Context, path *FieldPath, tag string, value reflect.Value, errs *ValidationErrors) {
	if !value.IsValid() {
		return
	}

	if tag != "" {
		tagRules := v.parseTagRules(tag)

		for _, tagRule := range tagRules {
			rule, exists := v.rules[tagRule.name]
			if !exists {
				continue
			}

			err := rule(ctx, value.Interface(), tagRule.params...)
			if err != nil {
				err.Path = path.JSONPointer()
				errs.Add(*err)
			}
		}
	}

	kind := value.Kind()
	if kind == reflect.Interface || kind == reflect.Ptr {
		if value.IsNil() {
			return
		}
		value = value.Elem()
		kind = value.Kind()
	}

	switch kind {
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			item := value.Index(i)
			itemPath := path.AppendIndex(i)
			v.validateValue(ctx, itemPath, "", item, errs)
		}

	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			k, val := iter.Key(), iter.Value()
			keyStr := formatMapKey(k)
			itemPath := path.AppendKey(keyStr)
			v.validateValue(ctx, itemPath, "", val, errs)
		}

	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			fieldValue := value.Field(i)
			fieldType := value.Type().Field(i)

			if fieldType.PkgPath != "" {
				continue
			}

			fieldName := getJSONFieldName(fieldType)
			fieldTag := fieldType.Tag.Get(ValidationTagName)
			fieldPath := path.AppendField(fieldName)

			v.validateValue(ctx, fieldPath, fieldTag, fieldValue, errs)
		}
	}
}

type tagRule struct {
	name   string
	params []string
}

func (v *Validator) parseTagRules(tag string) []tagRule {
	if tag == "" || tag == "-" {
		return nil
	}

	options := strings.Split(tag, ",")
	rules := make([]tagRule, 0, len(options))

	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}

		parts := strings.Split(option, " ")
		rules = append(rules, tagRule{
			name:   parts[0],
			params: parts[1:],
		})
	}

	return rules
}

func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return field.Name
	}

	name := strings.SplitN(tag, ",", 2)[0]
	if name == "" {
		return field.Name
	}

	return name
}

func formatMapKey(key reflect.Value) string {
	return key.String()
}

func getSimpleTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Bool:
		return "bool"
	case reflect.String:
		return "string"
	default:
		return t.String()
	}
}
