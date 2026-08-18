package reflect

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type mergePatchNested struct {
	Name  string `config:"name"`
	Count int    `config:"count"`
}

type mergePatchInline struct {
	Mode string `config:"mode"`
}

type mergePatchTarget struct {
	Nested   *mergePatchNested           `config:"nested"`
	Inline   mergePatchInline            `config:",inline"`
	Labels   map[string]string           `config:"labels"`
	Services map[string]mergePatchNested `config:"services"`
	Items    []mergePatchNested          `config:"items"`
	Timeout  time.Duration               `config:"timeout"`
}

type mergePatchNestedInput struct {
	Name  string `config:"name,omitempty"`
	Count int    `config:"count,omitempty"`
}

type mergePatchInput struct {
	Nested *mergePatchNestedInput `config:"nested"`
	Inline mergePatchInline       `config:",inline"`
	Labels map[string]any         `config:"labels,omitempty"`
	Items  []mergePatchNested     `config:"items,omitempty"`
}

type mergePatchDecoder struct {
	Value   string `config:"value"`
	Decoder string `config:"-"`
}

func (value *mergePatchDecoder) UnmarshalJSON(data []byte) error {
	value.Value = string(data)
	value.Decoder = "json"
	return nil
}

func (value *mergePatchDecoder) UnmarshalText(data []byte) error {
	value.Value = string(data)
	value.Decoder = "text"
	return nil
}

type mergePatchString string

func (value *mergePatchString) UnmarshalJSON([]byte) error {
	*value = "json"
	return nil
}

func (value *mergePatchString) UnmarshalText([]byte) error {
	*value = "text"
	return nil
}

type mergePatchTextDecoder struct {
	Value string
}

func (value *mergePatchTextDecoder) UnmarshalText(data []byte) error {
	value.Value = string(data)
	return nil
}

type mergePatchFailingDecoder struct {
	Decoder string
}

func (value *mergePatchFailingDecoder) UnmarshalJSON([]byte) error {
	value.Decoder = "json"
	return nil
}

func (value *mergePatchFailingDecoder) UnmarshalText([]byte) error {
	value.Decoder = "text"
	return errors.New("invalid text value")
}

func TestMergePatchUpdatesStructuredValue(t *testing.T) {
	target := mergePatchTarget{
		Nested: &mergePatchNested{Name: "default", Count: 1},
		Labels: map[string]string{"kept": "value", "removed": "value"},
		Services: map[string]mergePatchNested{
			"api": {Name: "default", Count: 1},
		},
	}
	patch := map[string]any{
		"nested": map[string]any{"name": "patched"},
		"mode":   "active",
		"labels": map[string]any{"added": "value", "removed": nil},
		"services": map[string]any{
			"api":    map[string]any{"count": "2"},
			"worker": map[string]any{"name": "worker"},
		},
		"items":   []any{map[string]any{"name": "item", "count": "3"}},
		"timeout": "1m30s",
	}
	if err := MergePatch(&target, patch, Options{TagNames: []string{"config"}}); err != nil {
		t.Fatal(err)
	}
	want := mergePatchTarget{
		Nested: &mergePatchNested{Name: "patched", Count: 1},
		Inline: mergePatchInline{Mode: "active"},
		Labels: map[string]string{"kept": "value", "added": "value"},
		Services: map[string]mergePatchNested{
			"api":    {Name: "default", Count: 2},
			"worker": {Name: "worker"},
		},
		Items:   []mergePatchNested{{Name: "item", Count: 3}},
		Timeout: 90 * time.Second,
	}
	if !reflect.DeepEqual(target, want) {
		t.Fatalf("target = %#v, want %#v", target, want)
	}
}

func TestMergePatchAcceptsStructAndArrayPatches(t *testing.T) {
	target := mergePatchTarget{
		Nested: &mergePatchNested{Name: "default", Count: 1},
		Labels: map[string]string{"kept": "value", "removed": "value"},
		Items:  []mergePatchNested{{Name: "kept"}},
	}
	patch := mergePatchInput{
		Nested: &mergePatchNestedInput{Count: 2},
		Inline: mergePatchInline{Mode: "active"},
		Labels: map[string]any{"added": "value", "removed": nil},
	}
	if err := MergePatch(&target, patch, Options{TagNames: []string{"config"}}); err != nil {
		t.Fatal(err)
	}
	want := mergePatchTarget{
		Nested: &mergePatchNested{Name: "default", Count: 2},
		Inline: mergePatchInline{Mode: "active"},
		Labels: map[string]string{"kept": "value", "added": "value"},
		Items:  []mergePatchNested{{Name: "kept"}},
	}
	if !reflect.DeepEqual(target, want) {
		t.Fatalf("target = %#v, want %#v", target, want)
	}

	array := [2]int{1, 2}
	if err := MergePatch(&array, []string{"3", "4"}, Options{}); err != nil {
		t.Fatal(err)
	}
	if array != [2]int{3, 4} {
		t.Fatalf("array = %#v, want [2]int{3, 4}", array)
	}
}

func TestMergePatchUsesTextBeforeJSONForStringPatch(t *testing.T) {
	target := mergePatchDecoder{}
	if err := MergePatch(&target, `{"value":"decoded"}`, Options{}); err != nil {
		t.Fatal(err)
	}
	if target.Decoder != "text" || target.Value != `{"value":"decoded"}` {
		t.Fatalf("target = %#v", target)
	}

	stringTarget := mergePatchString("default")
	if err := MergePatch(&stringTarget, "plain", Options{}); err != nil {
		t.Fatal(err)
	}
	if stringTarget != "plain" {
		t.Fatalf("string target = %q, want plain", stringTarget)
	}

	textTarget := mergePatchTextDecoder{}
	if err := MergePatch(&textTarget, "decoded", Options{}); err != nil {
		t.Fatal(err)
	}
	if textTarget.Value != "decoded" {
		t.Fatalf("text target = %#v", textTarget)
	}

	failingTarget := mergePatchFailingDecoder{}
	if err := MergePatch(&failingTarget, "invalid", Options{}); err == nil {
		t.Fatal("text decoding error was ignored")
	}
	if failingTarget.Decoder != "text" {
		t.Fatalf("decoder = %q, want text", failingTarget.Decoder)
	}
}

func TestMergePatchUsesStructureInsteadOfJSONDecoderForObjectPatch(t *testing.T) {
	target := mergePatchDecoder{Value: "default"}
	patch := map[string]any{"value": "structured"}
	if err := MergePatch(&target, patch, Options{TagNames: []string{"config"}}); err != nil {
		t.Fatal(err)
	}
	if target.Decoder != "" || target.Value != "structured" {
		t.Fatalf("target = %#v", target)
	}
}

func TestMergePatchMergesStructPatchIntoDynamicObject(t *testing.T) {
	type labelsPatch struct {
		Added string `config:"added"`
	}

	value := any(map[string]string{"kept": "value"})
	if err := MergePatch(&value, labelsPatch{Added: "value"}, Options{TagNames: []string{"config"}}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"kept": "value", "added": "value"}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("value = %#v, want %#v", value, want)
	}

	var empty any
	patch := []string{"one", "two"}
	if err := MergePatch(&empty, patch, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(empty, patch) {
		t.Fatalf("empty interface = %#v, want %#v", empty, patch)
	}
}

func TestMergePatchClearsNullableValues(t *testing.T) {
	target := mergePatchTarget{
		Nested: &mergePatchNested{Name: "value"},
		Labels: map[string]string{"removed": "value"},
	}
	patch := map[string]any{
		"nested": nil,
		"labels": map[string]any{"removed": nil},
	}
	if err := MergePatch(&target, patch, Options{TagNames: []string{"config"}}); err != nil {
		t.Fatal(err)
	}
	if target.Nested != nil || len(target.Labels) != 0 {
		t.Fatalf("target = %#v", target)
	}
}

func TestMergePatchReportsFieldErrors(t *testing.T) {
	target := mergePatchTarget{}
	err := MergePatch(&target, map[string]any{"nested": map[string]any{"count": "invalid"}}, Options{TagNames: []string{"config"}})
	var valueError *MergePatchError
	if !errors.As(err, &valueError) || valueError.Path != "nested.count" {
		t.Fatalf("error = %#v", err)
	}
	err = MergePatch(&target, map[string]any{"missing": true}, Options{TagNames: []string{"config"}})
	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) || unknown.Path != "missing" {
		t.Fatalf("error = %#v", err)
	}
}

func TestMergePatchRejectsEncodedStructures(t *testing.T) {
	target := mergePatchTarget{Labels: map[string]string{"kept": "value"}}
	for _, patch := range []map[string]any{
		{"labels": `{"added":"value"}`},
		{"items": `[{"name":"item","count":2}]`},
	} {
		if err := MergePatch(&target, patch, Options{TagNames: []string{"config"}}); err == nil {
			t.Fatalf("encoded structure patch %#v was accepted", patch)
		}
	}
}
