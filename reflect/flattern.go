package reflect

import "reflect"

// FlattenStruct iterates over fields of a struct, calling fieldFunc for each field.
// If a field is a struct, it will be recursively iterated until maxDepth is reached.
// If maxDepth is 0, only the top-level fields will be iterated.
// name is json path of the field. eg: "metadata.name"
func FlattenStruct(name string, maxDepth int, v reflect.Value, fieldFunc func(name string, v reflect.Value) error) error {
	if maxDepth == 0 {
		return fieldFunc(name, v)
	}
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	nextkey := func(next string) string {
		if name == "" {
			return next
		}
		return name + "." + next
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			isEmbedded, isIgnore, fieldName := StructFieldInfo(t.Field(i))
			if isIgnore {
				continue
			}
			fieldValue := v.Field(i)
			if isEmbedded {
				if err := FlattenStruct(name, maxDepth, fieldValue, fieldFunc); err != nil {
					return err
				}
				continue
			}
			if err := FlattenStruct(nextkey(fieldName), maxDepth-1, fieldValue, fieldFunc); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if err := fieldFunc(name, v); err != nil {
			return err
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if err := FlattenStruct(nextkey(key.String()), maxDepth-1, v.MapIndex(key), fieldFunc); err != nil {
				return err
			}
		}
	default:
		if err := fieldFunc(name, v); err != nil {
			return err
		}
	}
	return nil
}
