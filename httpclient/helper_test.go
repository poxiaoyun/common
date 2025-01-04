package httpclient

import (
	"net/url"
	"reflect"
	"testing"
)

type TestQeuryOption struct {
	Foo  string `json:"foo"`
	ABC  string `json:"abc,omitempty"`
	Bar  string
	Bool bool     `yaml:"bool"`
	Json JsonData `json:"json"`
}

type JsonData struct {
	Foo string `json:"foo"`
}

func TestObjectToQuery(t *testing.T) {
	tests := []struct {
		name string
		args any
		want url.Values
	}{
		{
			args: TestQeuryOption{
				Foo:  "foo",
				Bar:  "bar",
				Bool: true,
				Json: JsonData{Foo: "foo"},
			},
			want: url.Values{
				"foo":  []string{"foo"},
				"Bar":  []string{"bar"},
				"bool": []string{"true"},
				"json": []string{"{\"foo\":\"foo\"}"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ObjectToQuery(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObjectToQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
