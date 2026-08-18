package reflect

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// StructFieldInfo returns the field name of the struct field
func SetFiledValue(dest any, jsonpath string, value any) error {
	return setFieldValue(reflect.ValueOf(dest), value, parseJsonPath(jsonpath)...)
}

func parseJsonPath(jsonpath string) []string {
	pathes := []string{}
	for elem := range strings.SplitSeq(jsonpath, ".") {
		if elem != "" {
			if i := strings.IndexRune(elem, '['); i != -1 {
				path0, path1 := elem[:i], elem[i+1:]
				if j := strings.IndexRune(path1, ']'); j != -1 {
					path1 = path1[:j]
				}
				if path1 != "" {
					pathes = append(pathes, path0, path1)
					continue
				}
			}
			pathes = append(pathes, elem)
		}
	}
	return pathes
}

// GetFiledValue returns the field value of the struct field
func GetFiledValue(dest any, jsonpath string) (any, error) {
	return getFiledValue(reflect.ValueOf(dest), parseJsonPath(jsonpath)...)
}

func getFiledValue(v reflect.Value, path ...string) (any, error) {
	if len(path) == 0 {
		return v.Interface(), nil
	}
	switch t := v.Type(); t.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return nil, fmt.Errorf("nil pointer")
		}
		return getFiledValue(v.Elem(), path...)
	case reflect.Slice:
		if v.IsNil() {
			return nil, fmt.Errorf("nil slice")
		}
		index := path[0]
		if index == "*" {
			result := []any{}
			for i := 0; i < v.Len(); i++ {
				if val, err := getFiledValue(v.Index(i), path[1:]...); err == nil {
					result = append(result, val)
				}
			}
			return result, nil
		} else {
			i, err := strconv.Atoi(index)
			if err != nil {
				return nil, fmt.Errorf("invalid array index %s", index)
			}
			if i > v.Len() {
				return nil, fmt.Errorf("array index %d out of range", i)
			}
			return getFiledValue(v.Index(i), path[1:]...)
		}
	case reflect.Map:
		if v.IsNil() {
			return nil, fmt.Errorf("nil map")
		}
		key := reflect.ValueOf(path[0])
		if val := v.MapIndex(key); val.IsValid() {
			return getFiledValue(val, path[1:]...)
		}
		return nil, fmt.Errorf("key %s not found", path[0])
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			isEmbedded, isIgnore, fieldName := StructFieldInfo(field)
			if isIgnore {
				continue
			}
			if isEmbedded {
				if val, err := getFiledValue(v.Field(i), path...); err == nil {
					return val, nil
				}
				continue
			}
			if path[0] == fieldName {
				return getFiledValue(v.Field(i), path[1:]...)
			}
		}
		return nil, fmt.Errorf("field %s not found", path[0])
	default:
		return nil, fmt.Errorf("invalid type %v", t)
	}
}

func setFieldValue(v reflect.Value, value any, path ...string) error {
	if len(path) == 0 {
		return SetValue(v, value)
	}
	switch t := v.Type(); t.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(t.Elem()))
		}
		return setFieldValue(v.Elem(), value, path...)
	case reflect.Slice:
		if v.IsNil() {
			v.Set(reflect.MakeSlice(t, 0, 0))
		}
		index := path[0]
		if index == "*" {
			for i := 0; i < v.Len(); i++ {
				if err := setFieldValue(v.Index(i), value, path[1:]...); err != nil {
					return err
				}
			}
			return nil
		} else {
			i, err := strconv.Atoi(index)
			if err != nil {
				return fmt.Errorf("invalid array index %s", index)
			}
			if i > v.Len() {
				return fmt.Errorf("array index %d out of range", i)
			}
			return setFieldValue(v.Index(i), value, path[1:]...)
		}
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
		key, val := reflect.ValueOf(path[0]), reflect.New(t.Elem()).Elem()
		if exists := v.MapIndex(key); exists.IsValid() {
			val.Set(exists) // copy value
		}
		if err := setFieldValue(val, value, path[1:]...); err != nil {
			return err
		}
		v.SetMapIndex(key, val)
		return nil
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			isEmbedded, isIgnore, fieldName := StructFieldInfo(field)
			if isIgnore {
				continue
			}
			if isEmbedded {
				if err := setFieldValue(v.Field(i), value, path...); err != nil {
					continue
				}
				return nil
			}
			if path[0] == fieldName {
				return setFieldValue(v.Field(i), value, path[1:]...)
			}
		}
		return fmt.Errorf("field %s not found", path[0])
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func StructFieldInfo(structField reflect.StructField) (bool, bool, string) {
	isEmbedded, isIgnored, _, fieldName := StructFieldInfoN(structField)
	return isEmbedded, isIgnored, fieldName
}

func StructFieldInfoN(structField reflect.StructField) (bool, bool, bool, string) {
	return StructFieldInfoNByTags(structField, "json", "yaml")
}

// StructFieldInfoNByTags returns struct field metadata using the first present
// tag from tagNames.
func StructFieldInfoNByTags(structField reflect.StructField, tagNames ...string) (bool, bool, bool, string) {
	isEmbedded, isIgnored, fieldName, omitempty := structField.Anonymous, false, structField.Name, false
	fieldTag := ""
	for _, tagName := range tagNames {
		if tag, ok := structField.Tag.Lookup(tagName); ok {
			fieldTag = tag
			break
		}
	}
	if fieldTag != "" {
		opts := strings.Split(fieldTag, ",")

		jsonname, opts := opts[0], opts[1:]
		switch jsonname {
		case "-":
			isIgnored = true
		case "":
		default:
			fieldName = jsonname
			isEmbedded = false // if field is embedded,but json tag has name,then it is not embedded
		}

		for _, opt := range opts {
			switch opt {
			case "omitempty":
				omitempty = true
			case "inline": // inline is a json tag option
				isEmbedded = true
			}
		}
	}
	return isEmbedded, isIgnored, omitempty, fieldName
}

// IndirectType returns the first non-pointer type.
func IndirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// IndirectValue returns the first non-pointer value.
func IndirectValue(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

// IndirectValueAlloc returns the first non-pointer value and allocates nil
// pointers along the way.
func IndirectValueAlloc(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	return value
}

// IsNilable reports whether values of t can be nil.
func IsNilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

// WalkStructFields calls fieldFunc for every queryable leaf field in t.
// Tags are checked in order. Anonymous and inline structs are flattened,
// named structs use dot-separated paths, and outer fields shadow embedded
// fields with the same name.
func WalkStructFields(
	t reflect.Type,
	fieldFunc func(name string, index []int, omitEmpty bool) error,
	tagNames ...string,
) error {
	return walkStructFields(t, nil, "", tagNames, map[string]struct{}{}, map[reflect.Type]bool{}, fieldFunc)
}

func walkStructFields(
	t reflect.Type,
	index []int,
	prefix string,
	tagNames []string,
	seen map[string]struct{},
	stack map[reflect.Type]bool,
	fieldFunc func(name string, index []int, omitEmpty bool) error,
) error {
	t = IndirectType(t)
	if t.Kind() != reflect.Struct {
		return nil
	}
	if stack[t] {
		return nil
	}
	stack[t] = true
	defer delete(stack, t)

	type nestedField struct {
		typ    reflect.Type
		index  []int
		prefix string
	}
	nested := make([]nestedField, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		inline, ignored, fieldOmitEmpty, name := StructFieldInfoNByTags(field, tagNames...)
		if ignored {
			continue
		}
		fieldIndex := append(append([]int(nil), index...), i)
		fieldType := IndirectType(field.Type)
		if inline && fieldType.Kind() == reflect.Struct {
			nested = append(nested, nestedField{typ: field.Type, index: fieldIndex, prefix: prefix})
			continue
		}
		fieldName := name
		if prefix != "" {
			fieldName = prefix + "." + name
		}
		if fieldType.Kind() == reflect.Struct && !ImplementsJSONUnmarshaler(field.Type) && !ImplementsTextUnmarshaler(field.Type) {
			nested = append(nested, nestedField{typ: field.Type, index: fieldIndex, prefix: fieldName})
			continue
		}
		if _, exists := seen[fieldName]; exists {
			continue
		}
		seen[fieldName] = struct{}{}
		if err := fieldFunc(fieldName, fieldIndex, fieldOmitEmpty); err != nil {
			return err
		}
	}
	for _, field := range nested {
		if err := walkStructFields(field.typ, field.index, field.prefix, tagNames, seen, stack, fieldFunc); err != nil {
			return err
		}
	}
	return nil
}

// FieldByIndexAlloc follows a struct field index path and allocates nil
// pointers encountered along the way.
func FieldByIndexAlloc(value reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		value = IndirectValueAlloc(value)
		value = value.Field(i)
	}
	return value
}

// FieldByIndex follows a struct field index path without allocating pointers.
// It returns false when a nil pointer or interface is encountered.
func FieldByIndex(value reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if !value.IsValid() || value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		value = value.Field(i)
	}
	return value, value.IsValid()
}
