package collections

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
)

type Any struct {
	List  []Any
	Dict  OrderedMap[string, Any]
	Value any
}

func (a Any) MarshalJSON() ([]byte, error) {
	if a.List != nil {
		return json.Marshal(a.List)
	}
	if a.Dict != nil {
		return json.Marshal(a.Dict)
	}
	return json.Marshal(a.Value)
}

func (a *Any) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		a.List = nil
		a.Dict = nil
		a.Value = nil
		return nil
	}
	if data[0] == '[' {
		a.List = []Any{}
		return json.Unmarshal(data, &a.List)
	}
	if data[0] == '{' {
		a.Dict = OrderedMap[string, Any]{}
		return json.Unmarshal(data, &a.Dict)
	}
	return json.Unmarshal(data, &a.Value)
}

type OrderedMap[K comparable, V any] []OrderedMapEntry[K, V]

func (m OrderedMap[K, V]) Len() int {
	return len(m)
}

func (m OrderedMap[K, V]) Get(key K) (V, bool) {
	var zero V
	for _, entry := range m {
		if entry.Key == key {
			return entry.Value, true
		}
	}
	return zero, false
}

func (m *OrderedMap[K, V]) Set(key K, value V) {
	for i, entry := range *m {
		if entry.Key == key {
			(*m)[i].Value = value
			return
		}
	}
	*m = append(*m, OrderedMapEntry[K, V]{Key: key, Value: value})
}

func (m *OrderedMap[K, V]) Delete(key K) {
	for i, entry := range *m {
		if entry.Key == key {
			*m = slices.Delete(*m, i, i+1)
			return
		}
	}
}

func (m OrderedMap[K, V]) Keys() []K {
	keys := make([]K, len(m))
	for i, entry := range m {
		keys[i] = entry.Key
	}
	return keys
}

func (m OrderedMap[K, V]) Values() []V {
	values := make([]V, len(m))
	for i, entry := range m {
		values[i] = entry.Value
	}
	return values
}

func (m OrderedMap[K, V]) ToMap() map[K]V {
	result := make(map[K]V, len(m))
	for _, entry := range m {
		result[entry.Key] = entry.Value
	}
	return result
}

func (m OrderedMap[K, V]) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, kv := range m {
		if i != 0 {
			buf.WriteString(",")
		}
		key, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteString(":")
		val, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteString("}")
	return buf.Bytes(), nil
}

func (m *OrderedMap[K, V]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*m = nil
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return &json.UnmarshalTypeError{Value: "object", Type: reflect.TypeOf(m)}
	}
	// must not nil slice, non-nil slice is not a nil map
	entries := []OrderedMapEntry[K, V]{}
	for dec.More() {
		// Read key
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(K)
		if !ok {
			return &json.UnmarshalTypeError{
				Value:  "non-comparable key",
				Type:   reflect.TypeOf(key),
				Struct: "OrderedMap",
				Field:  "Key",
			}
		}
		var value V
		if err := dec.Decode(&value); err != nil {
			return err
		}
		entries = append(entries, OrderedMapEntry[K, V]{Key: key, Value: value})
	}
	*m = entries
	return nil
}

type OrderedMapEntry[K comparable, V any] struct {
	Key   K
	Value V
}
