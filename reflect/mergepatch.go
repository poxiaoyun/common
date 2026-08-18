package reflect

import (
	"fmt"
	"reflect"
	"sort"
)

// UnknownFieldError reports a patch field that does not exist in the target
// structure selected by Options.TagNames.
type UnknownFieldError struct {
	Path string
}

func (err *UnknownFieldError) Error() string {
	return fmt.Sprintf("unknown field %q", err.Path)
}

// MergePatchError reports a value that could not be applied at Path.
type MergePatchError struct {
	Path string
	Err  error
}

func (err *MergePatchError) Error() string {
	if err.Path == "" {
		return err.Err.Error()
	}
	return fmt.Sprintf("field %q: %v", err.Path, err.Err)
}

func (err *MergePatchError) Unwrap() error {
	return err.Err
}

// MergePatch recursively applies patch to the value referenced by target.
// Structs and string-keyed maps are object patches, struct field names follow
// Options.TagNames, objects merge recursively, nil deletes map entries, and
// slices and arrays are replaced. A string patch is decoded as text or JSON
// only when a non-string target explicitly implements the corresponding
// unmarshaler; text takes precedence over JSON.
func MergePatch(target, patch any, options Options) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("merge patch target must be a non-nil pointer")
	}
	options = resolveOptions(options)
	return mergePatchValue(value.Elem(), reflect.ValueOf(patch), options, "")
}

func mergePatchValue(target, patch reflect.Value, options Options, path string) error {
	patch = indirectPatchValue(patch)
	if !patch.IsValid() {
		if IsNilable(target.Type()) {
			target.SetZero()
			return nil
		}
		return mergePatchValueError(path, fmt.Errorf("cannot set nil to %v", target.Type()))
	}
	if patch.Kind() == reflect.String && IndirectType(target.Type()).Kind() != reflect.String &&
		(ImplementsJSONUnmarshaler(target.Type()) || ImplementsTextUnmarshaler(target.Type())) {
		if err := SetValue(target, patch.Interface()); err != nil {
			return mergePatchValueError(path, err)
		}
		return nil
	}
	if target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return mergePatchValue(target.Elem(), patch, options, path)
	}
	switch target.Kind() {
	case reflect.Struct:
		return mergePatchStruct(target, patch, options, path)
	case reflect.Map:
		return mergePatchMap(target, patch, options, path)
	case reflect.Slice, reflect.Array:
		return mergePatchCollection(target, patch, options, path)
	case reflect.Interface:
		return mergePatchInterface(target, patch, options, path)
	default:
		if err := SetValue(target, patch.Interface()); err != nil {
			return mergePatchValueError(path, err)
		}
		return nil
	}
}

func mergePatchStruct(target, patch reflect.Value, options Options, path string) error {
	object, ok := patchObject(patch, options)
	if !ok {
		return mergePatchValueError(path, fmt.Errorf("requires an object"))
	}
	for _, property := range object {
		index, exists := patchStructField(target.Type(), property.Name, options.TagNames)
		fieldPath := joinPatchField(path, property.Name)
		if !exists {
			return &UnknownFieldError{Path: fieldPath}
		}
		field := FieldByIndexAlloc(target, index)
		if err := mergePatchValue(field, property.Value, options, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func mergePatchMap(target, patch reflect.Value, options Options, path string) error {
	if target.Type().Key().Kind() != reflect.String {
		return mergePatchValueError(path, fmt.Errorf("map key type %v is not supported", target.Type().Key()))
	}
	object, ok := patchObject(patch, options)
	if !ok {
		return mergePatchValueError(path, fmt.Errorf("requires an object"))
	}
	if target.IsNil() {
		target.Set(reflect.MakeMap(target.Type()))
	}
	for _, property := range object {
		key := reflect.ValueOf(property.Name).Convert(target.Type().Key())
		patchValue := property.Value
		if !indirectPatchValue(patchValue).IsValid() {
			target.SetMapIndex(key, reflect.Value{})
			continue
		}
		element := reflect.New(target.Type().Elem()).Elem()
		if current := target.MapIndex(key); current.IsValid() {
			element.Set(current)
		}
		if err := mergePatchValue(element, patchValue, options, joinPatchMapKey(path, property.Name)); err != nil {
			return err
		}
		target.SetMapIndex(key, element)
	}
	return nil
}

func mergePatchCollection(target, patch reflect.Value, options Options, path string) error {
	if patch.Kind() != reflect.Slice && patch.Kind() != reflect.Array {
		return mergePatchValueError(path, fmt.Errorf("requires an array"))
	}
	if target.Kind() == reflect.Array && patch.Len() != target.Len() {
		return mergePatchValueError(path, fmt.Errorf("requires %d items", target.Len()))
	}
	if target.Kind() == reflect.Array {
		target.SetZero()
		for index := 0; index < patch.Len(); index++ {
			if err := mergePatchValue(target.Index(index), patch.Index(index), options, joinPatchIndex(path, index)); err != nil {
				return err
			}
		}
		return nil
	}
	result := reflect.MakeSlice(target.Type(), patch.Len(), patch.Len())
	for index := 0; index < patch.Len(); index++ {
		if err := mergePatchValue(result.Index(index), patch.Index(index), options, joinPatchIndex(path, index)); err != nil {
			return err
		}
	}
	target.Set(result)
	return nil
}

func mergePatchInterface(target, patch reflect.Value, options Options, path string) error {
	if target.NumMethod() != 0 {
		if err := SetValue(target, patch.Interface()); err != nil {
			return mergePatchValueError(path, err)
		}
		return nil
	}
	if target.IsNil() {
		target.Set(patch)
		return nil
	}
	current := reflect.New(target.Elem().Type()).Elem()
	current.Set(target.Elem())
	if _, object := patchObject(patch, options); object && (current.Kind() == reflect.Map || current.Kind() == reflect.Struct) {
		if err := mergePatchValue(current, patch, options, path); err != nil {
			return err
		}
		target.Set(current)
		return nil
	}
	target.Set(patch)
	return nil
}

func patchStructField(typ reflect.Type, name string, tagNames []string) ([]int, bool) {
	inline := []reflect.StructField{}
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		isInline, ignored, _, fieldName := StructFieldInfoNByTags(field, tagNames...)
		if ignored {
			continue
		}
		if isInline && IndirectType(field.Type).Kind() == reflect.Struct {
			inline = append(inline, field)
			continue
		}
		if fieldName == name {
			return field.Index, true
		}
	}
	for _, field := range inline {
		child, exists := patchStructField(IndirectType(field.Type), name, tagNames)
		if exists {
			return append(append([]int(nil), field.Index...), child...), true
		}
	}
	return nil, false
}

type patchProperty struct {
	Name  string
	Value reflect.Value
}

func patchObject(value reflect.Value, options Options) ([]patchProperty, bool) {
	value = indirectPatchValue(value)
	if !value.IsValid() {
		return nil, false
	}
	if value.Kind() == reflect.Map {
		if value.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		properties := make([]patchProperty, 0, value.Len())
		for _, key := range value.MapKeys() {
			properties = append(properties, patchProperty{Name: key.String(), Value: value.MapIndex(key)})
		}
		sortPatchProperties(properties)
		return properties, true
	}
	if value.Kind() != reflect.Struct {
		return nil, false
	}
	node := ParseStruct(value.Interface(), options)
	if node.Kind != ObjectNode {
		return nil, false
	}
	properties := make([]patchProperty, 0, len(node.Fields))
	for _, field := range node.Fields {
		properties = append(properties, patchProperty{Name: field.Name, Value: field.Value})
	}
	sortPatchProperties(properties)
	return properties, true
}

func indirectPatchValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func sortPatchProperties(properties []patchProperty) {
	sort.SliceStable(properties, func(i, j int) bool {
		return properties[i].Name < properties[j].Name
	})
}

func mergePatchValueError(path string, err error) error {
	return &MergePatchError{Path: path, Err: err}
}

func joinPatchField(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func joinPatchMapKey(path, key string) string {
	return path + "[" + key + "]"
}

func joinPatchIndex(path string, index int) string {
	return fmt.Sprintf("%s[%d]", path, index)
}
