package api_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

func TestStrictBody(t *testing.T) {
	type input struct {
		Name   string `json:"name"`
		Config struct {
			Enabled bool `json:"enabled"`
		} `json:"config"`
	}
	for _, test := range []struct {
		name, body, media string
		status            int32
	}{
		{"valid", `{"name":"account","config":{"enabled":true}} `, "application/json; charset=utf-8", 0},
		{"unknown", `{"name":"account","organization":"foreign"}`, "application/json", 422},
		{"nested unknown", `{"config":{"ignored":true}}`, "application/json", 422},
		{"type mismatch", `{"name":42}`, "application/json", 422},
		{"syntax", `{"name":`, "application/json", 400},
		{"empty", ``, "application/json", 400},
		{"trailing value", `{"name":"account"} {}`, "application/json", 400},
		{"trailing garbage", `{"name":"account"} x`, "application/json", 400},
		{"wrong media", `name: account`, "application/yaml", 415},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.media)
			var value input
			err := api.Body(r, &value, api.WithStrictBody())
			if test.status != 0 {
				status, ok := err.(*commonerrors.Status)
				if !ok || status.Code != test.status {
					t.Fatalf("error = %v, want status %d", err, test.status)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if value.Name != "account" || !value.Config.Enabled {
				t.Fatalf("decoded = %+v", value)
			}
		})
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write([]byte(`{"name":"compressed"}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", &compressed)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Content-Encoding", "gzip")
	var value input
	if err := api.Body(r, &value, api.WithStrictBody()); err != nil {
		t.Fatal(err)
	}
	if value.Name != "compressed" {
		t.Fatalf("decoded = %+v", value)
	}

	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"permissive","unknown":1}`))
	r.Header.Set("Content-Type", "application/json")
	if err := api.Body(r, &value); err != nil {
		t.Fatal(err)
	}
	if value.Name != "permissive" {
		t.Fatalf("decoded = %+v", value)
	}
}

type queryMode string

type upperText string

func (u *upperText) UnmarshalText(text []byte) error {
	*u = upperText(strings.ToUpper(string(text)))
	return nil
}

type queryObjectFilter struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type queryObjectJSON struct {
	Name string `json:"name"`
}

func (v *queryObjectJSON) UnmarshalJSON(data []byte) error {
	type plain queryObjectJSON
	return json.Unmarshal(data, (*plain)(v))
}

type queryObjectOptions struct {
	meta.ListOptions

	Mode       queryMode          `json:"mode"`
	Published  *bool              `json:"published"`
	Count      int                `json:"count"`
	Labels     []string           `json:"label"`
	Last       string             `json:"last"`
	Fallback   string             `json:"fallback"`
	Custom     upperText          `json:"custom"`
	Filter     *queryObjectFilter `json:"filter"`
	Payload    queryObjectJSON    `json:"payload"`
	Raw        json.RawMessage    `json:"raw"`
	Ignored    string             `json:"-"`
	NoTag      string
	unexported string
}

func TestQueryObject(t *testing.T) {
	request := httptest.NewRequest(
		"GET",
		"/clusters?page=2&size=25&mode=Kubernetes&published=false&count=3"+
			"&label=one&label=two&last=first&last=second&fallback=json-name&custom=mixed"+
			"&filter.name=demo&filter.enabled=true"+
			"&payload=%7B%22name%22%3A%22json%22%7D&raw=%7B%22raw%22%3Atrue%7D"+
			"&NoTag=plain&unexported=hidden&unknown=value",
		nil,
	)

	options, err := api.QueryObject[queryObjectOptions](request)
	if err != nil {
		t.Fatal(err)
	}
	if options.Page != 2 || options.Size != 25 {
		t.Fatalf("unexpected embedded list options: %#v", options.ListOptions)
	}
	if options.Mode != "Kubernetes" {
		t.Fatalf("mode = %q", options.Mode)
	}
	if options.Published == nil || *options.Published {
		t.Fatalf("published = %#v, want pointer to false", options.Published)
	}
	if options.Count != 3 {
		t.Fatalf("count = %d, want 3", options.Count)
	}
	if len(options.Labels) != 2 || options.Labels[0] != "one" || options.Labels[1] != "two" {
		t.Fatalf("labels = %#v", options.Labels)
	}
	if options.Last != "second" {
		t.Fatalf("last = %q, want second", options.Last)
	}
	if options.Fallback != "json-name" {
		t.Fatalf("fallback = %q", options.Fallback)
	}
	if options.Custom != "mixed" {
		t.Fatalf("custom = %q, want mixed", options.Custom)
	}
	if options.Filter == nil || options.Filter.Name != "demo" || !options.Filter.Enabled {
		t.Fatalf("filter = %#v", options.Filter)
	}
	if options.Payload.Name != "json" {
		t.Fatalf("payload = %#v", options.Payload)
	}
	if string(options.Raw) != `{"raw":true}` {
		t.Fatalf("raw = %s", options.Raw)
	}
	if options.NoTag != "plain" {
		t.Fatalf("NoTag = %q, want plain", options.NoTag)
	}
	if options.unexported != "" {
		t.Fatalf("unexported = %q, want empty", options.unexported)
	}
}

func TestQueryObjectLeavesMissingPointersNil(t *testing.T) {
	request := httptest.NewRequest("GET", "/clusters", nil)
	options, err := api.QueryObject[queryObjectOptions](request)
	if err != nil {
		t.Fatal(err)
	}
	if options.Published != nil {
		t.Fatalf("published = %#v, want nil", options.Published)
	}
}

func TestQueryObjectRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"/clusters?published=invalid",
		"/clusters?count=invalid",
		"/clusters?payload=invalid-json",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest("GET", target, nil)
			_, err := api.QueryObject[queryObjectOptions](request)
			status, ok := err.(*commonerrors.Status)
			if !ok || status.Code != 400 {
				t.Fatalf("error = %#v, want bad request status", err)
			}
		})
	}
}

func TestQueryObjectRejectsNonStruct(t *testing.T) {
	request := httptest.NewRequest("GET", "/clusters?value=1", nil)
	if _, err := api.QueryObject[int](request); err != nil {
		return
	}
	t.Fatal("expected an error")
}

func TestStrictQueryObject(t *testing.T) {
	type Embedded struct {
		Limit int `query:"limit"`
	}
	type options struct {
		Embedded
		Position   *int64             `query:"atPosition"`
		Dimensions map[string]string  `query:"dimensions"`
		Filter     *queryObjectFilter `query:"filter"`
	}
	request := httptest.NewRequest("GET", "/accounts?limit=2&atPosition=0&dimensions=%7B%22organization%22%3A%22a%22%7D&filter.name=bank", nil)
	got, err := api.QueryObject[options](request, api.WithStrictQuery())
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 2 || got.Position == nil || *got.Position != 0 || got.Dimensions["organization"] != "a" || got.Filter == nil || got.Filter.Name != "bank" {
		t.Fatalf("decoded query: %+v", got)
	}
	for _, query := range []string{"limt=2", "limit=bad", "filter.missing=x", "limit=%ZZ", "limit=1;bad=2"} {
		request := httptest.NewRequest("GET", "/accounts?"+query, nil)
		_, err := api.QueryObject[options](request, api.WithStrictQuery())
		if !commonerrors.IsCode(err, 400) {
			t.Fatalf("query %s: %v", query, err)
		}
	}
	empty, err := api.QueryObject[options](httptest.NewRequest("GET", "/accounts", nil), api.WithStrictQuery())
	if err != nil {
		t.Fatal(err)
	}
	if empty.Position != nil || empty.Filter != nil {
		t.Fatal("omitted pointers were allocated")
	}
}

func TestStrictProtocolJSON(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	for _, test := range []struct {
		media, body string
		code        int32
	}{
		{"application/cloudevents+json; charset=utf-8", `{"value":"accepted"}`, 0},
		{"application/json", `{"value":"accepted"}`, 415},
		{"application/cloudevents+json", `{"unknown":1}`, 422},
		{"application/cloudevents+json", `{"value":"accepted"} {}`, 400},
	} {
		var got input
		err := api.ReadContent(test.media, "", strings.NewReader(test.body), &got, api.WithStrictBody(), api.WithJSONMediaType("application/cloudevents+json"))
		if err != nil {
			if test.code == 0 || !commonerrors.IsCode(err, int(test.code)) {
				t.Fatalf("decode: %v", err)
			}
			continue
		}
		if test.code != 0 || got.Value != "accepted" {
			t.Fatalf("unexpected acceptance: %+v", test)
		}
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"value":"compressed"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/events", &compressed)
	r.Header.Set("Content-Type", "application/cloudevents+json")
	r.Header.Set("Content-Encoding", "gzip")
	var got input
	if err := api.Body(r, &got, api.WithStrictBody(), api.WithJSONMediaType("application/cloudevents+json")); err != nil {
		t.Fatal(err)
	}
	if got.Value != "compressed" {
		t.Fatal("compressed protocol body was not decoded")
	}
}
