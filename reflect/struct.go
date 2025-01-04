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
	isEmbedded, isIgnored, fieldName, omitempty := structField.Anonymous, false, structField.Name, false
	// json
	fieldTag := structField.Tag.Get("json")
	if fieldTag == "" {
		fieldTag = structField.Tag.Get("yaml")
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

func SetValueAutoConvert(v reflect.Value, value any) error {
	newv := reflect.ValueOf(value)
	if v.CanSet() && newv.Type().AssignableTo(v.Type()) {
		v.Set(newv)
		return nil
	}
	switch newv.Kind() {
	case reflect.String:
		return SetStringAutoConvert(v, newv.String())
	default:
		return fmt.Errorf("can not set value %v to %v", newv.Type(), v.Type())
	}
}

// SetStringAutoConvert sets string value to reflect.Value
// It will auto convert string to target type
func SetStringAutoConvert(v reflect.Value, str string) error {
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
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(str, 64)
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
	default:
		return fmt.Errorf("can not set string to %v", v.Type())
	}
	return nil
}
