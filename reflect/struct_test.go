package reflect

import (
	"encoding/json"
	"reflect"
	"testing"
)

type Foo struct {
	Name string `json:"name"`
}

type Bar struct {
	Baz string `json:"baz"`
}

type Embedded struct {
	Foo   `json:",inline"`
	List  []Bar             `json:"list"`
	KV    map[string]string `json:"kv"`
	Items map[string]Bar    `json:"items"`
}

type TaggedEmbedded struct {
	Value  string `json:"value"`
	Shadow string `query:"same"`
}

type TaggedNested struct {
	Enabled bool `query:"enabled"`
}

type TaggedFilter struct {
	Name   string        `query:"name"`
	Nested *TaggedNested `query:"nested"`
}

type TaggedJSON struct {
	Name string `json:"name"`
}

func (v *TaggedJSON) UnmarshalJSON(data []byte) error {
	type plain TaggedJSON
	return json.Unmarshal(data, (*plain)(v))
}

type taggedStruct struct {
	*TaggedEmbedded
	Shadow   string        `query:"same"`
	Fallback string        `json:"fallback"`
	Ignored  string        `query:"-"`
	Filter   *TaggedFilter `query:"filter"`
	JSON     TaggedJSON    `query:"json"`
}

type recursiveTaggedStruct struct {
	Name  string                 `json:"name"`
	Child *recursiveTaggedStruct `json:"child"`
}

func TestWalkStructFieldsStopsRecursiveTypes(t *testing.T) {
	var names []string
	err := WalkStructFields(reflect.TypeFor[recursiveTaggedStruct](), func(name string, _ []int, _ bool) error {
		names = append(names, name)
		return nil
	}, "query", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"name"}) {
		t.Fatalf("field names = %#v, want []string{\"name\"}", names)
	}
}

func TestWalkStructFields(t *testing.T) {
	type walkedField struct {
		name  string
		index []int
	}
	var fields []walkedField
	err := WalkStructFields(reflect.TypeFor[taggedStruct](), func(name string, index []int, _ bool) error {
		fields = append(fields, walkedField{name: name, index: index})
		return nil
	}, "query", "json")
	if err != nil {
		t.Fatal(err)
	}
	fieldIndex := func(name string) []int {
		for _, field := range fields {
			if field.name == name {
				return field.index
			}
		}
		return nil
	}
	if got := fieldIndex("same"); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("outer field path = %#v, want []int{1}", got)
	}
	if got := fieldIndex("value"); !reflect.DeepEqual(got, []int{0, 0}) {
		t.Fatalf("embedded field path = %#v, want []int{0, 0}", got)
	}
	if got := fieldIndex("fallback"); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("fallback field path = %#v, want []int{2}", got)
	}
	if got := fieldIndex("Ignored"); got != nil {
		t.Fatal("ignored field was collected")
	}
	if got := fieldIndex("filter.name"); !reflect.DeepEqual(got, []int{4, 0}) {
		t.Fatalf("nested field path = %#v, want []int{4, 0}", got)
	}
	if got := fieldIndex("filter.nested.enabled"); !reflect.DeepEqual(got, []int{4, 1, 0}) {
		t.Fatalf("deeply nested field path = %#v, want []int{4, 1, 0}", got)
	}
	if got := fieldIndex("json"); !reflect.DeepEqual(got, []int{5}) {
		t.Fatalf("JSON unmarshaler field path = %#v, want []int{5}", got)
	}
	if got := fieldIndex("json.name"); got != nil {
		t.Fatal("JSON unmarshaler was expanded as a nested struct")
	}

	value := taggedStruct{}
	field := FieldByIndexAlloc(reflect.ValueOf(&value).Elem(), fieldIndex("value"))
	field.SetString("set")
	if value.TaggedEmbedded == nil || value.Value != "set" {
		t.Fatalf("embedded pointer was not allocated: %#v", value)
	}
	nested := FieldByIndexAlloc(reflect.ValueOf(&value).Elem(), fieldIndex("filter.nested.enabled"))
	nested.SetBool(true)
	if value.Filter == nil || value.Filter.Nested == nil || !value.Filter.Nested.Enabled {
		t.Fatalf("nested pointers were not allocated: %#v", value)
	}
	jsonField := FieldByIndexAlloc(reflect.ValueOf(&value).Elem(), fieldIndex("json"))
	if err := SetStringAutoConvert(jsonField, `{"name":"decoded"}`); err != nil {
		t.Fatal(err)
	}
	if value.JSON.Name != "decoded" {
		t.Fatalf("JSON unmarshaler result = %#v", value.JSON)
	}
}

func TestSetValueAutoConvertSlices(t *testing.T) {
	type customInt int

	ints := []customInt{}
	if err := SetSliceAutoConvert(reflect.ValueOf(&ints).Elem(), reflect.ValueOf([]int{1, 2})); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ints, []customInt{1, 2}) {
		t.Fatalf("ints = %#v", ints)
	}

	var last *int
	if err := SetValueAutoConvert(reflect.ValueOf(&last).Elem(), []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if last == nil || *last != 2 {
		t.Fatalf("last = %#v", last)
	}

	bools := []bool{}
	if err := SetValueAutoConvert(reflect.ValueOf(&bools).Elem(), []string{"true", "false"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bools, []bool{true, false}) {
		t.Fatalf("bools = %#v", bools)
	}

	array := [3]int{}
	if err := SetValueAutoConvert(reflect.ValueOf(&array).Elem(), []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if array != [3]int{1, 2, 0} {
		t.Fatalf("array = %#v", array)
	}
}

func TestSetFiledValue(t *testing.T) {
	type args struct {
		dest     any
		jsonpath string
		value    any
	}
	tests := []struct {
		name    string
		args    args
		want    any
		wantErr bool
	}{
		{
			name: "set struct field",
			args: args{
				dest:     &Embedded{},
				jsonpath: ".name",
				value:    "hello",
			},
			want: &Embedded{
				Foo: Foo{Name: "hello"},
			},
		},
		{
			name: "set list item",
			args: args{
				dest:     &Embedded{List: []Bar{{}}},
				jsonpath: ".list[0].baz",
				value:    "baz",
			},
			want: &Embedded{List: []Bar{{Baz: "baz"}}},
		},
		{
			name: "set map value",
			args: args{
				dest:     &Embedded{},
				jsonpath: ".kv.hello",
				value:    "world",
			},
			want: &Embedded{KV: map[string]string{"hello": "world"}},
		},
		{
			name: "set map value 2",
			args: args{
				dest:     &Embedded{},
				jsonpath: ".items.hello.baz",
				value:    "world",
			},
			want: &Embedded{Items: map[string]Bar{"hello": {Baz: "world"}}},
		},
		{
			name: "update map value",
			args: args{
				dest:     &Embedded{Items: map[string]Bar{"hello": {Baz: "foo"}}},
				jsonpath: ".items.hello.baz",
				value:    "world",
			},
			want: &Embedded{Items: map[string]Bar{"hello": {Baz: "world"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetFiledValue(tt.args.dest, tt.args.jsonpath, tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetFiledValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.args.dest, tt.want) {
				t.Errorf("SetFiledValue() got = %v, want %v", tt.args.dest, tt.want)
			}
		})
	}
}

func TestGetFiledValue(t *testing.T) {
	type args struct {
		dest     any
		jsonpath string
	}
	tests := []struct {
		name    string
		args    args
		want    any
		wantErr bool
	}{
		{
			name: "get struct field",
			args: args{
				dest:     &Embedded{Foo: Foo{Name: "hello"}},
				jsonpath: ".name",
			},
			want: "hello",
		},
		{
			name: "get list item",
			args: args{
				dest:     &Embedded{List: []Bar{{Baz: "baz"}}},
				jsonpath: ".list[0].baz",
			},
			want: "baz",
		},
		{
			name: "get map value",
			args: args{
				dest:     &Embedded{KV: map[string]string{"hello": "world"}},
				jsonpath: ".kv.hello",
			},
			want: "world",
		},
		{
			name: "get map value 2",
			args: args{
				dest:     &Embedded{Items: map[string]Bar{"hello": {Baz: "world"}}},
				jsonpath: ".items.hello.baz",
			},
			want: "world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetFiledValue(tt.args.dest, tt.args.jsonpath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFiledValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetFiledValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getFiledValue(t *testing.T) {
	type args struct {
		v    reflect.Value
		path []string
	}
	tests := []struct {
		name    string
		args    args
		want    any
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getFiledValue(tt.args.v, tt.args.path...)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFiledValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFiledValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
