package meta

import (
	"strconv"
	"strings"
	"time"
)

// Or returns the first non-zero value from the given values
// example:
//
//	val := Or(a, b, c)
func Or[T comparable](vals ...T) T {
	var zero T
	for _, v := range vals {
		if v != zero {
			return v
		}
	}
	return zero
}

// Tenary returns v1 if cond is true, otherwise returns v2
func Tenary[T any](cond bool, v1, v2 T) T {
	if cond {
		return v1
	}
	return v2
}

// DerefPtr dereferences a pointer, if the pointer is nil, returns the default value
func DerefPtr[T any](ptr *T, defaultVal T) T {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// Ptr returns a pointer to the given value
func Ptr[T any](v T) *T {
	return &v
}

// ParseString return the parsed value or the default value if parsing fails
//
// example:
//
//	intval := ParseString[int]("123", 0) // intval is 123
//	isOk := ParseString[bool]("true", false) // isOk is true
//	strlist := ParseString[[]string]("a,b,c", nil) // strlist is []string{"a", "b", "c"}
//	timeval := ParseString[time.Time]("2023-10-01T12:00:00Z", time.Time{}) // timeval is time.Time value
func ParseString[T any](str string, def T) T {
	if str == "" {
		return def
	}
	switch any(def).(type) {
	case string:
		return any(str).(T)
	case []string:
		parts := strings.Split(str, ",")
		// trim spaces around elements and filter empty items
		var out []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, p)
		}
		return any(out).(T)
	case int:
		if v, err := strconv.Atoi(str); err == nil {
			return any(v).(T)
		}
	case bool:
		if v, err := strconv.ParseBool(str); err == nil {
			return any(v).(T)
		}
	case int64:
		if v, err := strconv.ParseInt(str, 10, 64); err == nil {
			return any(v).(T)
		}
	case uint64:
		if v, err := strconv.ParseUint(str, 10, 64); err == nil {
			return any(v).(T)
		}
	case uint32:
		if v, err := strconv.ParseUint(str, 10, 32); err == nil {
			return any(uint32(v)).(T)
		}
	case float64:
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			return any(v).(T)
		}
	case time.Time:
		if v, err := time.Parse(time.RFC3339, str); err == nil {
			return any(v).(T)
		}
	case time.Duration:
		if v, err := time.ParseDuration(str); err == nil {
			return any(v).(T)
		}
	case *int:
		if v, err := strconv.Atoi(str); err == nil {
			return any(&v).(T)
		}
	case *int64:
		if v, err := strconv.ParseInt(str, 10, 64); err == nil {
			return any(&v).(T)
		}
	case *bool:
		if v, err := strconv.ParseBool(str); err == nil {
			return any(&v).(T)
		}
	}
	return def
}
