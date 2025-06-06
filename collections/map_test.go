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
		{
			name:    "empty list",
			data:    "null",
			want:    Any{List: nil},
			wantErr: false,
		},
		{
			name: "non-empty list",
			data: `[null,{"key":[]}]`,
			want: Any{
				List: []Any{
					{},
					{Dict: OrderedMap[string, Any]{{Key: "key", Value: Any{List: []Any{}}}}},
				},
			},
			wantErr: false,
		},
		{
			name: "ordered map",
			data: `{"key1": "value1", "key2": {"subkey": "subvalue"}}`,
			want: Any{
				Dict: OrderedMap[string, Any]{
					{Key: "key1", Value: Any{Value: "value1"}},
					{Key: "key2", Value: Any{
						Dict: OrderedMap[string, Any]{
							{Key: "subkey", Value: Any{Value: "subvalue"}},
						},
					}},
				},
			},
		},
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
			name: "empty map",
			m:    nil,
			want: "null",
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
