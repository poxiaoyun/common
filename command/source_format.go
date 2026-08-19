package command

import (
	"reflect"
	"time"

	libreflect "xiaoshiai.cn/common/reflect"
)

func isZeroConfigurationData(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Map, reflect.Slice:
		return reflected.Len() == 0
	default:
		return reflected.IsZero()
	}
}

func configurationTypeName(typ reflect.Type) string {
	typ = libreflect.IndirectType(typ)
	if typ == reflect.TypeFor[time.Duration]() {
		return "duration"
	}
	if typ == reflect.TypeFor[time.Time]() {
		return "RFC3339"
	}
	switch typ.Kind() {
	case reflect.Slice:
		return "list"
	case reflect.Map:
		return "json"
	default:
		return typ.Kind().String()
	}
}
