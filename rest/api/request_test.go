package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

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
		"/clusters?page=2&size=25&continue=next-token&mode=Kubernetes&published=false&count=3"+
			"&label=one&label=two&last=first&last=second&fallback=json-name&custom=mixed"+
			"&filter.name=demo&filter.enabled=true"+
			"&payload=%7B%22name%22%3A%22json%22%7D&raw=%7B%22raw%22%3Atrue%7D"+
			"&NoTag=plain&unexported=hidden&unknown=value",
		nil,
	)

	options, err := QueryObject[queryObjectOptions](request)
	if err != nil {
		t.Fatal(err)
	}
	if options.Page != 2 || options.Size != 25 || options.Continue != "next-token" {
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
	options, err := QueryObject[queryObjectOptions](request)
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
			_, err := QueryObject[queryObjectOptions](request)
			if err == nil {
				t.Fatal("expected an error")
			}
			status, ok := err.(*commonerrors.Status)
			if !ok || status.Code != 400 {
				t.Fatalf("error = %#v, want bad request status", err)
			}
		})
	}
}

func TestQueryObjectRejectsNonStruct(t *testing.T) {
	request := httptest.NewRequest("GET", "/clusters?value=1", nil)
	if _, err := QueryObject[int](request); err == nil {
		t.Fatal("expected an error")
	}
}
