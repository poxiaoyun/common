package jsonschema

import (
	"encoding/json"
	"testing"
)

func TestEffectiveComposition(t *testing.T) {
	schema := decodeSchema(t, `{
  "type": "object",
  "if": {"properties": {"mode": {"const": "advanced"}}},
  "then": {"properties": {"advanced": {"type": "boolean"}}},
  "else": {"properties": {"simple": {"type": "boolean"}}},
  "allOf": [{"required": ["mode"]}],
  "anyOf": [
    {"properties": {"mode": {"const": "advanced"}}, "title": "Advanced"},
    {"properties": {"mode": {"const": "simple"}}, "title": "Simple"}
  ]
}`)
	result, err := Effective(schema, schema, map[string]any{"mode": "advanced"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Advanced" || len(result.Required) != 1 || result.Required[0] != "mode" {
		t.Fatalf("effective schema = %#v", result)
	}
	if property(result, "advanced") == nil || property(result, "simple") != nil {
		t.Fatalf("properties = %#v", result.Properties)
	}
	if result.If != nil || len(result.AllOf) != 0 || len(result.AnyOf) != 0 {
		t.Fatalf("composition keywords were retained: %#v", result)
	}
}

func TestEffectiveOneOfRequiresUniqueMatch(t *testing.T) {
	schema := decodeSchema(t, `{
  "oneOf": [
    {"type": "integer", "title": "first"},
    {"type": "integer", "title": "second"}
  ]
}`)
	result, err := Effective(schema, schema, int64(1))
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "" {
		t.Fatalf("ambiguous oneOf selected %q", result.Title)
	}
}

func TestEffectiveResolvesLocalReferenceAndMergesProperties(t *testing.T) {
	root := decodeSchema(t, `{
  "$defs": {
    "worker": {
      "type": "object",
      "required": ["count"],
      "properties": {"count": {"type": "integer", "minimum": 1}}
    }
  },
  "properties": {
    "worker": {
      "allOf": [
        {"$ref": "#/$defs/worker"},
        {"required": ["enabled"], "properties": {"count": {"maximum": 8}}}
      ]
    }
  }
}`)
	worker := property(root, "worker")
	result, err := Effective(root, *worker, map[string]any{"count": int64(2), "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Required) != 2 {
		t.Fatalf("required = %#v", result.Required)
	}
	count := property(result, "count")
	if count == nil || count.Minimum == nil || count.Maximum == nil || *count.Minimum != 1 || *count.Maximum != 8 {
		t.Fatalf("count schema = %#v", count)
	}
}

func TestEffectiveConditionKeepsRootDefinitions(t *testing.T) {
	root := decodeSchema(t, `{
  "$defs": {"advanced": {"const": "advanced"}},
  "if": {"properties": {"mode": {"$ref": "#/$defs/advanced"}}},
  "then": {"title": "Advanced"},
  "else": {"title": "Simple"}
}`)
	result, err := Effective(root, root, map[string]any{"mode": "advanced"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Advanced" {
		t.Fatalf("title = %q", result.Title)
	}
}

func TestMergeIntersectsTypes(t *testing.T) {
	base := decodeSchema(t, `{"type":["string","number"]}`)
	override := decodeSchema(t, `{"type":"string"}`)
	result := Merge(base, override)
	if len(result.Type) != 1 || result.Type[0] != "string" {
		t.Fatalf("type = %#v", result.Type)
	}
}

func TestSchemaNormalizesBooleanSchemas(t *testing.T) {
	valid := decodeSchema(t, `true`)
	if err := Validate(valid, "anything"); err != nil {
		t.Fatalf("true schema rejected value: %v", err)
	}
	invalid := decodeSchema(t, `false`)
	if err := Validate(invalid, "anything"); err == nil {
		t.Fatal("false schema accepted value")
	}
}

func decodeSchema(t *testing.T, value string) Schema {
	t.Helper()
	result := Schema{}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func property(schema Schema, name string) *Schema {
	for i := range schema.Properties {
		if schema.Properties[i].Name == name {
			return &schema.Properties[i].Schema
		}
	}
	return nil
}
