package collections

import (
	"reflect"
	"testing"
)

func TestAnyMarshal(t *testing.T) {
	tests := []struct {
		name string
		a    Any
		want string
	}{
		{
			name: "empty list",
			a:    Any{List: nil},
			want: "null",
		},
		{
			name: "non-empty list",
			a: Any{
				List: []Any{
					{},
					{Dict: OrderedMap[string, Any]{
						{Key: "key", Value: Any{List: []Any{}}},
					}},
				},
			},
			want: `[null,{"key":[]}]`,
		},
		{
			name: "empty dict",
			a:    Any{Dict: OrderedMap[string, Any]{}},
			want: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.MarshalJSON()
			if err != nil {
				t.Errorf("Any.MarshalJSON() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("Any.MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAnyUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    Any
		wantErr bool
	}{
		// {
		// 	name:    "null value",
		// 	data:    "null",
		// 	want:    Any{},
		// 	wantErr: false,
		// },
		// {
		// 	name: "non-empty list",
		// 	data: `[null,{"key":[]}]`,
		// 	want: Any{
		// 		List: []Any{
		// 			{},
		// 			{Dict: OrderedMap[string, Any]{{Key: "key", Value: Any{List: []Any{}}}}},
		// 		},
		// 	},
		// 	wantErr: false,
		// },
		// {
		// 	name: "ordered map",
		// 	data: `{"key1": "value1", "key2": {"subkey": "subvalue"}}`,
		// 	want: Any{
		// 		Dict: OrderedMap[string, Any]{
		// 			{Key: "key1", Value: Any{Value: "value1"}},
		// 			{Key: "key2", Value: Any{
		// 				Dict: OrderedMap[string, Any]{
		// 					{Key: "subkey", Value: Any{Value: "subvalue"}},
		// 				},
		// 			}},
		// 		},
		// 	},
		// },
		// {
		// 	name:    "empty list",
		// 	data:    "[]",
		// 	want:    Any{List: []Any{}},
		// 	wantErr: false,
		// },
		{
			name:    "empty dict",
			data:    "{}",
			want:    Any{Dict: OrderedMap[string, Any]{}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Any
			err := got.UnmarshalJSON([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("Any.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Any.UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedMapMarshal(t *testing.T) {
	tests := []struct {
		name string
		m    OrderedMap[string, int]
		want string
	}{
		{
			name: "nil map",
			m:    nil,
			want: "null",
		},
		{
			name: "empty map",
			m:    OrderedMap[string, int]{},
			want: "{}",
		},
		{
			name: "single entry",
			m:    OrderedMap[string, int]{{Key: "a", Value: 1}},
			want: `{"a":1}`,
		},
		{
			name: "multiple entries",
			m:    OrderedMap[string, int]{{Key: "a", Value: 1}, {Key: "b", Value: 2}},
			want: `{"a":1,"b":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.MarshalJSON()
			if err != nil {
				t.Errorf("OrderedMap.MarshalJSON() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("OrderedMap.MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOrderedMapUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    OrderedMap[string, int]
		wantErr bool
	}{
		{
			name:    "empty map",
			data:    "null",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "single entry",
			data:    `{"a":1}`,
			want:    OrderedMap[string, int]{{Key: "a", Value: 1}},
			wantErr: false,
		},
		{
			name:    "multiple entries",
			data:    `{"a":1,"b":2}`,
			want:    OrderedMap[string, int]{{Key: "a", Value: 1}, {Key: "b", Value: 2}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OrderedMap[string, int]
			err := got.UnmarshalJSON([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("OrderedMap.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OrderedMap.UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedMap_Methods(t *testing.T) {
	t.Run("Set and Get", func(t *testing.T) {
		var m OrderedMap[string, int]
		m.Set("a", 1)
		if m.Len() != 1 {
			t.Errorf("expected len 1, got %d", m.Len())
		}
		if v, ok := m.Get("a"); !ok || v != 1 {
			t.Errorf("expected key 'a' to be 1, got %d (ok=%v)", v, ok)
		}

		m.Set("b", 2)
		if m.Len() != 2 {
			t.Errorf("expected len 2, got %d", m.Len())
		}
		if v, ok := m.Get("b"); !ok || v != 2 {
			t.Errorf("expected key 'b' to be 2, got %d (ok=%v)", v, ok)
		}

		// Update existing
		m.Set("a", 3)
		if m.Len() != 2 {
			t.Errorf("expected len 2 after update, got %d", m.Len())
		}
		if v, ok := m.Get("a"); !ok || v != 3 {
			t.Errorf("expected key 'a' to be 3, got %d (ok=%v)", v, ok)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		m := OrderedMap[string, int]{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
			{Key: "c", Value: 3},
		}

		m.Delete("b")
		if m.Len() != 2 {
			t.Errorf("expected len 2, got %d", m.Len())
		}
		if _, ok := m.Get("b"); ok {
			t.Error("expected key 'b' to be deleted")
		}

		// Check order
		keys := m.Keys()
		if !reflect.DeepEqual(keys, []string{"a", "c"}) {
			t.Errorf("expected keys [a c], got %v", keys)
		}

		m.Delete("non-existent")
		if m.Len() != 2 {
			t.Errorf("expected len 2, got %d", m.Len())
		}
	})

	t.Run("Keys and Values", func(t *testing.T) {
		m := OrderedMap[string, int]{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
		}

		keys := m.Keys()
		if !reflect.DeepEqual(keys, []string{"a", "b"}) {
			t.Errorf("expected keys [a b], got %v", keys)
		}

		values := m.Values()
		if !reflect.DeepEqual(values, []int{1, 2}) {
			t.Errorf("expected values [1 2], got %v", values)
		}
	})

	t.Run("ToMap", func(t *testing.T) {
		m := OrderedMap[string, int]{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
		}

		stdMap := m.ToMap()
		if len(stdMap) != 2 {
			t.Errorf("expected map len 2, got %d", len(stdMap))
		}
		if stdMap["a"] != 1 || stdMap["b"] != 2 {
			t.Errorf("map content mismatch: %v", stdMap)
		}
	})
}

func TestOrderedMap_JSONErrors(t *testing.T) {
	t.Run("Marshal Error", func(t *testing.T) {
		m := OrderedMap[string, func()]{
			{Key: "a", Value: func() {}},
		}
		_, err := m.MarshalJSON()
		if err == nil {
			t.Error("expected error marshaling func, got nil")
		}
	})

	t.Run("Unmarshal Error - Invalid JSON", func(t *testing.T) {
		var m OrderedMap[string, int]
		err := m.UnmarshalJSON([]byte(`invalid`))
		if err == nil {
			t.Error("expected error unmarshaling invalid json, got nil")
		}
	})

	t.Run("Unmarshal Error - Not an Object", func(t *testing.T) {
		var m OrderedMap[string, int]
		err := m.UnmarshalJSON([]byte(`[]`))
		if err == nil {
			t.Error("expected error unmarshaling array, got nil")
		}
	})
}
