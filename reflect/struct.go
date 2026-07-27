package reflect

import (
	"encoding"
	"encoding/json"
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
	for _, elem := range strings.Split(jsonpath, ".") {
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
		return SetValueAutoConvert(v, value)
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

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
var jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()

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
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
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
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if inline && fieldType.Kind() == reflect.Struct {
			nested = append(nested, nestedField{typ: field.Type, index: fieldIndex, prefix: prefix})
			continue
		}
		fieldName := name
		if prefix != "" {
			fieldName = prefix + "." + name
		}
		if fieldType.Kind() == reflect.Struct && !ImplementsTextOrJSONUnmarshaler(field.Type) {
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

// ImplementsTextOrJSONUnmarshaler reports whether a type or its pointer
// implements encoding.TextUnmarshaler or json.Unmarshaler.
func ImplementsTextOrJSONUnmarshaler(t reflect.Type) bool {
	if t.Implements(textUnmarshalerType) || t.Implements(jsonUnmarshalerType) {
		return true
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	ptr := reflect.PointerTo(t)
	return ptr.Implements(textUnmarshalerType) || ptr.Implements(jsonUnmarshalerType)
}

// FieldByIndexAlloc follows a struct field index path and allocates nil
// pointers encountered along the way.
func FieldByIndexAlloc(value reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}
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

func SetValueAutoConvert(v reflect.Value, value any) error {
	return setValueAutoConvert(v, reflect.ValueOf(value))
}

func setValueAutoConvert(v, newv reflect.Value) error {
	if !newv.IsValid() {
		return fmt.Errorf("can not set nil to %v", v.Type())
	}
	for newv.Kind() == reflect.Interface {
		if newv.IsNil() {
			return fmt.Errorf("can not set nil to %v", v.Type())
		}
		newv = newv.Elem()
	}
	if v.CanSet() && newv.Type().AssignableTo(v.Type()) {
		v.Set(newv)
		return nil
	}
	switch newv.Kind() {
	case reflect.String:
		return SetStringAutoConvert(v, newv.String())
	case reflect.Slice, reflect.Array:
		return SetSliceAutoConvert(v, newv)
	case reflect.Pointer:
		if newv.IsNil() {
			return fmt.Errorf("can not set nil to %v", v.Type())
		}
		return setValueAutoConvert(v, newv.Elem())
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return setValueAutoConvert(v.Elem(), newv)
	}
	if newv.Type().ConvertibleTo(v.Type()) && newv.Kind() == v.Kind() {
		v.Set(newv.Convert(v.Type()))
		return nil
	}
	return fmt.Errorf("can not set value %v to %v", newv.Type(), v.Type())
}

// SetSliceAutoConvert converts a slice or array value into a target value.
// Collection targets consume every source element; scalar and unmarshaler
// targets consume the last source element.
func SetSliceAutoConvert(v, values reflect.Value) error {
	if !values.IsValid() || (values.Kind() != reflect.Slice && values.Kind() != reflect.Array) {
		return fmt.Errorf("source value must be a slice or array")
	}
	target := v
	for target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		target = target.Elem()
	}
	if values.Len() == 0 {
		switch target.Kind() {
		case reflect.Slice:
			target.Set(reflect.MakeSlice(target.Type(), 0, 0))
			return nil
		case reflect.Array:
			target.SetZero()
			return nil
		default:
			return fmt.Errorf("can not set empty %v to %v", values.Type(), v.Type())
		}
	}
	if ImplementsTextOrJSONUnmarshaler(v.Type()) {
		return setValueAutoConvert(v, values.Index(values.Len()-1))
	}
	switch target.Kind() {
	case reflect.Slice:
		slice := reflect.MakeSlice(target.Type(), values.Len(), values.Len())
		for i := 0; i < values.Len(); i++ {
			if err := setValueAutoConvert(slice.Index(i), values.Index(i)); err != nil {
				return err
			}
		}
		target.Set(slice)
		return nil
	case reflect.Array:
		if values.Len() > target.Len() {
			return fmt.Errorf("can not set %d values to %v", values.Len(), target.Type())
		}
		target.SetZero()
		for i := 0; i < values.Len(); i++ {
			if err := setValueAutoConvert(target.Index(i), values.Index(i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return setValueAutoConvert(v, values.Index(values.Len()-1))
	}
}

// SetStringAutoConvert sets string value to reflect.Value
// It will auto convert string to target type
func SetStringAutoConvert(v reflect.Value, str string) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if unmarshaler, ok := v.Interface().(encoding.TextUnmarshaler); ok {
			return unmarshaler.UnmarshalText([]byte(str))
		}
		if unmarshaler, ok := v.Interface().(json.Unmarshaler); ok {
			return unmarshaler.UnmarshalJSON([]byte(str))
		}
		return SetStringAutoConvert(v.Elem(), str)
	}
	if v.CanAddr() {
		if unmarshaler, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return unmarshaler.UnmarshalText([]byte(str))
		}
		if unmarshaler, ok := v.Addr().Interface().(json.Unmarshaler); ok {
			return unmarshaler.UnmarshalJSON([]byte(str))
		}
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(str)
	case reflect.Bool:
		n, err := strconv.ParseBool(str)
		if err != nil {
			return err
		}
		v.SetBool(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(str, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(str, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(str, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetFloat(n)
	case reflect.Slice:
		stringSlice := strings.Split(str, ",")
		slice := reflect.MakeSlice(v.Type(), len(stringSlice), len(stringSlice))
		for i, s := range stringSlice {
			if err := SetStringAutoConvert(slice.Index(i), s); err != nil {
				return err
			}
		}
		v.Set(slice)
	case reflect.Map:
		if !v.CanAddr() {
			return fmt.Errorf("can not address %v", v.Type())
		}
		if err := json.Unmarshal([]byte(str), v.Addr().Interface()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("can not set string to %v", v.Type())
	}
	return nil
}
