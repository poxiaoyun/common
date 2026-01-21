package validation

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// RuleFunc 规则函数类型
type RuleFunc func(ctx context.Context, value any, params ...string) *ValidationError

// DefaultRules 默认的规则集
var DefaultRules = map[string]RuleFunc{
	"required": requiredRule,
	"in":       inRule,
	"len":      lenRule,
	"min":      minRule,
	"max":      maxRule,
	"range":    rangeRule,
	"regexp":   regexpRule,
	"port":     portRule,
}

// requiredRule 必填字段验证
func requiredRule(ctx context.Context, value any, params ...string) *ValidationError {
	if value == nil {
		return NewValidationError("required", nil, nil)
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		if v.IsNil() {
			return NewValidationError("required", nil, nil)
		}
	}
	return nil
}

// rangeRule 数值范围验证
func rangeRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) != 2 {
		return NewValidationError("range", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	minVal, err1 := strconv.ParseFloat(params[0], 64)
	maxVal, err2 := strconv.ParseFloat(params[1], 64)
	if err1 != nil || err2 != nil {
		return NewValidationError("range", params, nil)
	}

	var actualVal float64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		actualVal = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		actualVal = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		actualVal = v.Float()
	default:
		return NewValidationError("range", params, nil)
	}

	if actualVal < minVal || actualVal > maxVal {
		return NewValidationError("range", params, actualVal)
	}

	return nil
}

// inRule 枚举值验证
func inRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) == 0 {
		return NewValidationError("in", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	var strValue string
	switch v.Kind() {
	case reflect.String:
		strValue = v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		strValue = strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		strValue = strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		strValue = strconv.FormatFloat(v.Float(), 'f', -1, 64)
	default:
		strValue = v.String()
	}

	for _, p := range params {
		if p == strValue {
			return nil
		}
	}

	return NewValidationError("in", params, strValue)
}

// lenRule 长度验证
// 支持以下格式:
//   - len 10       固定长度
//   - len 1 50     范围长度 (空格分隔)
//   - len 1-50     范围长度 (连字符分隔)
//   - len 0.1-1    比例范围 (用于百分比等场景)
func lenRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) == 0 || len(params) > 2 {
		return NewValidationError("len", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	var length int
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
		length = v.Len()
	default:
		return NewValidationError("len", params, nil)
	}

	// 解析参数
	var minVal, maxVal float64
	var isRange bool

	if len(params) == 1 {
		param := params[0]
		// 检查是否包含连字符 (范围格式: 1-50 或 0.1-1)
		if idx := strings.Index(param, "-"); idx > 0 {
			minStr := param[:idx]
			maxStr := param[idx+1:]
			var err1, err2 error
			minVal, err1 = strconv.ParseFloat(minStr, 64)
			maxVal, err2 = strconv.ParseFloat(maxStr, 64)
			if err1 != nil || err2 != nil {
				return NewValidationError("len", params, nil)
			}
			isRange = true
		} else {
			// 固定长度
			expected, err := strconv.Atoi(param)
			if err != nil {
				return NewValidationError("len", params, nil)
			}
			if length != expected {
				return NewValidationError("len", params, length)
			}
			return nil
		}
	} else {
		// 空格分隔的范围: len 1 50
		var err1, err2 error
		minVal, err1 = strconv.ParseFloat(params[0], 64)
		maxVal, err2 = strconv.ParseFloat(params[1], 64)
		if err1 != nil || err2 != nil {
			return NewValidationError("len", params, nil)
		}
		isRange = true
	}

	if isRange {
		lengthVal := float64(length)
		if lengthVal < minVal || lengthVal > maxVal {
			return NewValidationError("len", params, length)
		}
	}

	return nil
}

// minRule 最小值验证
func minRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) != 1 {
		return NewValidationError("min", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	minVal, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return NewValidationError("min", params, nil)
	}

	var actualVal float64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		actualVal = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		actualVal = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		actualVal = v.Float()
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		actualVal = float64(v.Len())
	default:
		return NewValidationError("min", params, nil)
	}

	if actualVal < minVal {
		return NewValidationError("min", params, actualVal)
	}

	return nil
}

// maxRule 最大值验证
func maxRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) != 1 {
		return NewValidationError("max", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	maxVal, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return NewValidationError("max", params, nil)
	}

	var actualVal float64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		actualVal = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		actualVal = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		actualVal = v.Float()
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		actualVal = float64(v.Len())
	default:
		return NewValidationError("max", params, nil)
	}

	if actualVal > maxVal {
		return NewValidationError("max", params, actualVal)
	}

	return nil
}

// regexpRule 正则表达式验证
func regexpRule(ctx context.Context, value any, params ...string) *ValidationError {
	if len(params) == 0 {
		return NewValidationError("regexp", params, nil)
	}

	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	if v.Kind() != reflect.String {
		return NewValidationError("regexp", params, nil)
	}

	pattern := strings.Join(params, " ")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return NewValidationError("regexp", params, nil)
	}

	strValue := v.String()
	if !re.MatchString(strValue) {
		return NewValidationError("regexp", params, sanitizeValue(strValue, 100))
	}

	return nil
}

// portRule 端口验证
func portRule(ctx context.Context, value any, params ...string) *ValidationError {
	v := getReflectValue(value)
	if !v.IsValid() {
		return nil
	}

	var port int64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		port = v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		port = int64(v.Uint())
	case reflect.String:
		p, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return NewValidationError("port", nil, v.String())
		}
		port = p
	default:
		return NewValidationError("port", nil, nil)
	}

	if port < 1 || port > 65535 {
		return NewValidationError("port", nil, port)
	}

	return nil
}

// getReflectValue 获取值的 reflect.Value，处理指针和接口
func getReflectValue(value any) reflect.Value {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	return v
}

// sanitizeValue 对值进行脱敏处理 (用于错误消息中显示)
func sanitizeValue(value any, maxLen int) any {
	if value == nil {
		return nil
	}

	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if len(s) > maxLen {
			return s[:maxLen] + "..."
		}
		return s
	case reflect.Slice, reflect.Array:
		if v.Len() > 10 {
			return "[...]"
		}
		return value
	case reflect.Map:
		if v.Len() > 10 {
			return "{...}"
		}
		return value
	default:
		return value
	}
}

// IsValidIP 验证 IP 地址
func IsValidIP(str string) bool {
	return IsIP(str)
}

// IsValidEnvName 验证环境变量名
func IsValidEnvName(str string) bool {
	if str == "" {
		return false
	}
	envNameRegexp := regexp.MustCompile(EnvNameRegexp)
	return envNameRegexp.MatchString(str)
}
