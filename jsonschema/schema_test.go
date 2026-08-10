package jsonschema

import (
	"encoding/json"
	"testing"
)

func TestPropertiesPreserveDeclarationOrder(t *testing.T) {
	var schema Schema
	if err := json.Unmarshal([]byte(`{
		"properties": {
			"second": {"type": "string", "x-order": 2},
			"first": {"type": "string", "x-order": 1}
		}
	}`), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) != 2 || schema.Properties[0].Name != "second" || schema.Properties[1].Name != "first" {
		t.Fatalf("properties = %#v", schema.Properties)
	}
}

func TestSchemaOrBoolRoundTrip(t *testing.T) {
	for _, value := range []string{
		`{"additionalProperties":true}`,
		`{"additionalProperties":false}`,
		`{"additionalProperties":{"type":"string"}}`,
	} {
		var schema Schema
		if err := json.Unmarshal([]byte(value), &schema); err != nil {
			t.Fatalf("unmarshal %s: %v", value, err)
		}
		data, err := json.Marshal(schema)
		if err != nil {
			t.Fatalf("marshal %s: %v", value, err)
		}
		if string(data) != value {
			t.Fatalf("round trip = %s, want %s", data, value)
		}
	}
}
