package config_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"xiaoshiai.cn/common/config"
	commonerrors "xiaoshiai.cn/common/errors"
)

type writeOptionFunc func(*config.WriteOptions)

func (option writeOptionFunc) ApplyToWrite(options *config.WriteOptions) {
	option(options)
}

func TestEncodeObject(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantError bool
	}{
		{name: "object", input: map[string]any{"enabled": true}},
		{name: "empty object", input: map[string]any{}},
		{name: "null", input: nil, wantError: true},
		{name: "array", input: []string{"one"}, wantError: true},
		{name: "scalar", input: "value", wantError: true},
		{name: "invalid raw JSON", input: json.RawMessage(`{"broken":`), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object, err := config.EncodeObject(test.input)
			if test.wantError {
				if !commonerrors.IsCode(err, http.StatusBadRequest) {
					t.Fatalf("EncodeObject() error = %v, want BadRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EncodeObject() error = %v", err)
			}
			if object == nil {
				t.Fatal("EncodeObject() returned nil")
			}
		})
	}
}

func TestObjectDecodeReplacesTargetAndPreservesNumbers(t *testing.T) {
	target := struct {
		Count   json.Number `json:"count"`
		Removed string      `json:"removed"`
	}{Removed: "old"}
	object := config.Object{"count": json.Number("9007199254740993")}
	if err := object.Decode(&target); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if target.Count != "9007199254740993" || target.Removed != "" {
		t.Fatalf("Decode() target = %#v", target)
	}
}

func TestObjectJSONDecodingPreservesNumbers(t *testing.T) {
	object := config.Object{}
	if err := json.Unmarshal([]byte(`{"count":9007199254740993}`), &object); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if object["count"] != json.Number("9007199254740993") {
		t.Fatalf("Unmarshal() count = %#v", object["count"])
	}
}

func TestWriteOptionsAreOpenAndAcceptMissingVersion(t *testing.T) {
	tests := []struct {
		name      string
		option    config.WriteOption
		want      int64
		wantError bool
	}{
		{name: "missing version", option: config.IfVersion(0), want: 0},
		{name: "persisted version", option: config.IfVersion(7), want: 7},
		{name: "open option", option: writeOptionFunc(func(options *config.WriteOptions) {
			version := int64(9)
			options.ExpectedVersion = &version
		}), want: 9},
		{name: "negative version", option: config.IfVersion(-1), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := config.ResolveWriteOptions(test.option)
			if test.wantError {
				if !commonerrors.IsCode(err, http.StatusBadRequest) {
					t.Fatalf("ResolveWriteOptions() error = %v, want BadRequest", err)
				}
				return
			}
			if err != nil || resolved.ExpectedVersion == nil || *resolved.ExpectedVersion != test.want {
				t.Fatalf("ResolveWriteOptions() = %#v, %v", resolved, err)
			}
		})
	}
}
