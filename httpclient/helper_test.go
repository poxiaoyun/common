package httpclient_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

type TestQeuryOption struct {
	Foo  string `json:"foo"`
	ABC  string `json:"abc,omitempty"`
	Bar  string
	Bool bool     `json:"bool"`
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
				"foo":      []string{"foo"},
				"Bar":      []string{"bar"},
				"bool":     []string{"true"},
				"json.foo": []string{"foo"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpclient.ObjectToQuery(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ObjectToQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListOptionsToQueryPagination(t *testing.T) {
	tests := []struct {
		name    string
		options meta.ListOptions
		want    url.Values
	}{
		{name: "mode neutral", want: url.Values{}},
		{
			name:    "page",
			options: meta.ListOptions{Page: 2, Size: 20},
			want:    url.Values{"page": []string{"2"}, "size": []string{"20"}},
		},
		{
			name:    "non-positive page and unrelated continuation pass through",
			options: meta.ListOptions{Page: -2, Size: 20, Continue: "ignored"},
			want:    url.Values{"continue": []string{"ignored"}, "page": []string{"-2"}, "size": []string{"20"}},
		},
		{
			name:    "first continuation batch",
			options: meta.ListOptions{Limit: 20},
			want:    url.Values{"limit": []string{"20"}},
		},
		{
			name:    "mixed pagination passes through",
			options: meta.ListOptions{Page: 2, Size: 10, Continue: "next", Limit: 20},
			want:    url.Values{"continue": []string{"next"}, "limit": []string{"20"}, "page": []string{"2"}, "size": []string{"10"}},
		},
		{
			name:    "negative batch values pass through",
			options: meta.ListOptions{Size: -10, Limit: -20},
			want:    url.Values{"limit": []string{"-20"}, "size": []string{"-10"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := httpclient.ListOptionsToQuery(test.options)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ListOptionsToQuery() = %#v, want %#v", got, test.want)
			}
		})
	}
}

type roundTripText string

func (v *roundTripText) MarshalText() ([]byte, error) {
	return []byte(*v), nil
}

func (v *roundTripText) UnmarshalText(data []byte) error {
	*v = roundTripText(data)
	return nil
}

type roundTripJSON struct {
	Name string `json:"name"`
}

func (v *roundTripJSON) MarshalJSON() ([]byte, error) {
	type plain roundTripJSON
	return json.Marshal(plain(*v))
}

func (v *roundTripJSON) UnmarshalJSON(data []byte) error {
	type plain roundTripJSON
	return json.Unmarshal(data, (*plain)(v))
}

type roundTripFilter struct {
	Name string `json:"name"`
}

type roundTripOptions struct {
	meta.ListOptions
	Filter     *roundTripFilter  `json:"filter,omitempty"`
	Labels     []int             `json:"label,omitempty"`
	Published  *bool             `json:"published,omitempty"`
	Text       roundTripText     `json:"text,omitempty"`
	Payload    roundTripJSON     `json:"payload"`
	Raw        json.RawMessage   `json:"raw"`
	Attributes map[string]string `json:"attributes,omitempty"`
	NoTag      string
	Ignored    string `json:"-"`
}

func TestObjectToQueryRoundTrip(t *testing.T) {
	published := false
	input := roundTripOptions{
		ListOptions: meta.ListOptions{Continue: "next-token", Limit: 20},
		Filter:      &roundTripFilter{Name: "demo"},
		Labels:      []int{1, 2},
		Published:   &published,
		Text:        "text",
		Payload:     roundTripJSON{Name: "json"},
		Raw:         json.RawMessage(`{"raw":true}`),
		Attributes:  map[string]string{"region": "cn"},
		NoTag:       "plain",
	}

	query := httpclient.ObjectToQuery(input)
	request := httptest.NewRequest("GET", "/?"+query.Encode(), nil)
	output, err := api.QueryObject[roundTripOptions](request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(output, input) {
		t.Fatalf("round trip result = %#v, want %#v; query = %s", output, input, query.Encode())
	}
}

type failingText string

func (*failingText) MarshalText() ([]byte, error) {
	return nil, errors.New("failed")
}

func (*failingText) UnmarshalText([]byte) error {
	return nil
}

func TestObjectToQueryIgnoresFieldError(t *testing.T) {
	input := struct {
		Value failingText `json:"value"`
		Good  string      `json:"good"`
	}{Good: "value"}
	query := httpclient.ObjectToQuery(input)
	if query.Get("value") != "" || query.Get("good") != "value" {
		t.Fatalf("query = %#v", query)
	}
	request := httpclient.Get("/").QueriesObject(input)
	if request.R.Err != nil || request.R.Queries.Get("good") != "value" {
		t.Fatalf("request = %#v", request.R)
	}
}
