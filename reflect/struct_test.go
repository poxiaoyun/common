package reflect

import (
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
