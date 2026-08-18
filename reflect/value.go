package reflect

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
var jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()

// FormatValue returns the textual representation of value. Strings remain
// unquoted, text marshalers define their representation, durations use Go
// duration syntax, and remaining values use JSON.
func FormatValue(value any) (string, error) {
	if value == nil {
		return "null", nil
	}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return "null", nil
		}
		reflected = reflected.Elem()
	}
	if reflected.Kind() == reflect.String {
		return reflected.String(), nil
	}
	if marshaler, exists := textMarshaler(reflect.ValueOf(value)); exists {
		text, err := marshaler.MarshalText()
		return string(text), err
	}
	if reflected.Type() == reflect.TypeFor[time.Duration]() {
		return time.Duration(reflected.Int()).String(), nil
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func textMarshaler(value reflect.Value) (encoding.TextMarshaler, bool) {
	if marshaler, exists := value.Interface().(encoding.TextMarshaler); exists {
		return marshaler, true
	}
	if value.Kind() != reflect.Pointer {
		pointer := reflect.New(value.Type())
		pointer.Elem().Set(value)
		marshaler, exists := pointer.Interface().(encoding.TextMarshaler)
		return marshaler, exists
	}
	return nil, false
}

// SetValue assigns value to target using semantic conversions for text and
// collections when direct assignment is not possible.
func SetValue(target reflect.Value, value any) error {
	return setValue(target, reflect.ValueOf(value))
}

func setValue(target, value reflect.Value) error {
	if !value.IsValid() {
		return fmt.Errorf("can not set nil to %v", target.Type())
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return fmt.Errorf("can not set nil to %v", target.Type())
		}
		value = value.Elem()
	}
	if target.CanSet() && value.Type().AssignableTo(target.Type()) {
		target.Set(value)
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		return SetTextValue(target, value.String())
	case reflect.Slice, reflect.Array:
		return setValues(target, value)
	case reflect.Pointer:
		if value.IsNil() {
			return fmt.Errorf("can not set nil to %v", target.Type())
		}
		return setValue(target, value.Elem())
	}
	if target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return setValue(target.Elem(), value)
	}
	if value.Type().ConvertibleTo(target.Type()) && value.Kind() == target.Kind() {
		target.Set(value.Convert(target.Type()))
		return nil
	}
	return fmt.Errorf("can not set value %v to %v", value.Type(), target.Type())
}

// SetTextValue parses text and assigns it to target. String targets remain raw,
// text unmarshaling takes precedence over JSON unmarshaling, and durations use
// Go duration syntax.
func SetTextValue(target reflect.Value, text string) error {
	if IndirectType(target.Type()).Kind() == reflect.String {
		IndirectValueAlloc(target).SetString(text)
		return nil
	}
	if target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		if unmarshaler, exists := target.Interface().(encoding.TextUnmarshaler); exists {
			return unmarshaler.UnmarshalText([]byte(text))
		}
		if unmarshaler, exists := target.Interface().(json.Unmarshaler); exists {
			return unmarshaler.UnmarshalJSON([]byte(text))
		}
		return SetTextValue(target.Elem(), text)
	}
	if target.CanAddr() {
		if unmarshaler, exists := target.Addr().Interface().(encoding.TextUnmarshaler); exists {
			return unmarshaler.UnmarshalText([]byte(text))
		}
		if unmarshaler, exists := target.Addr().Interface().(json.Unmarshaler); exists {
			return unmarshaler.UnmarshalJSON([]byte(text))
		}
	}
	if target.Type() == reflect.TypeFor[time.Duration]() {
		duration, err := time.ParseDuration(text)
		if err != nil {
			return err
		}
		target.SetInt(int64(duration))
		return nil
	}
	switch target.Kind() {
	case reflect.Bool:
		value, err := strconv.ParseBool(text)
		if err != nil {
			return err
		}
		target.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(text, 10, target.Type().Bits())
		if err != nil {
			return err
		}
		target.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(text, 10, target.Type().Bits())
		if err != nil {
			return err
		}
		target.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(text, target.Type().Bits())
		if err != nil {
			return err
		}
		target.SetFloat(value)
	case reflect.Slice:
		parts := strings.Split(text, ",")
		values := reflect.MakeSlice(target.Type(), len(parts), len(parts))
		for index, part := range parts {
			if err := SetTextValue(values.Index(index), part); err != nil {
				return err
			}
		}
		target.Set(values)
	case reflect.Map:
		return json.Unmarshal([]byte(text), target.Addr().Interface())
	default:
		return fmt.Errorf("can not set text to %v", target.Type())
	}
	return nil
}

// ImplementsTextUnmarshaler reports whether a type or its pointer implements
// encoding.TextUnmarshaler.
func ImplementsTextUnmarshaler(typ reflect.Type) bool {
	return implementsInterface(typ, textUnmarshalerType)
}

// ImplementsJSONUnmarshaler reports whether a type or its pointer implements
// json.Unmarshaler.
func ImplementsJSONUnmarshaler(typ reflect.Type) bool {
	return implementsInterface(typ, jsonUnmarshalerType)
}

func implementsInterface(typ, interfaceType reflect.Type) bool {
	if typ.Implements(interfaceType) {
		return true
	}
	typ = IndirectType(typ)
	return reflect.PointerTo(typ).Implements(interfaceType)
}

// Collection targets consume every source element; scalar and unmarshaler
// targets consume the last source element.
func setValues(target, values reflect.Value) error {
	if !values.IsValid() || values.Kind() != reflect.Slice && values.Kind() != reflect.Array {
		return fmt.Errorf("source value must be a slice or array")
	}
	indirect := IndirectValueAlloc(target)
	if values.Len() == 0 {
		switch indirect.Kind() {
		case reflect.Slice:
			indirect.Set(reflect.MakeSlice(indirect.Type(), 0, 0))
			return nil
		case reflect.Array:
			indirect.SetZero()
			return nil
		default:
			return fmt.Errorf("can not set empty %v to %v", values.Type(), target.Type())
		}
	}
	if ImplementsTextUnmarshaler(target.Type()) || ImplementsJSONUnmarshaler(target.Type()) {
		return setValue(target, values.Index(values.Len()-1))
	}
	switch indirect.Kind() {
	case reflect.Slice:
		result := reflect.MakeSlice(indirect.Type(), values.Len(), values.Len())
		for index := 0; index < values.Len(); index++ {
			if err := setValue(result.Index(index), values.Index(index)); err != nil {
				return err
			}
		}
		indirect.Set(result)
		return nil
	case reflect.Array:
		if values.Len() > indirect.Len() {
			return fmt.Errorf("can not set %d values to %v", values.Len(), indirect.Type())
		}
		indirect.SetZero()
		for index := 0; index < values.Len(); index++ {
			if err := setValue(indirect.Index(index), values.Index(index)); err != nil {
				return err
			}
		}
		return nil
	default:
		return setValue(target, values.Index(values.Len()-1))
	}
}
