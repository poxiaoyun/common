package store

import (
	"reflect"
	"testing"
)

func TestParseRequirements(t *testing.T) {
	tests := []struct {
		name    string
		want    Requirements
		wantErr bool
	}{
		{
			name: "empty",
			want: Requirements{},
		},
		{
			name: "single",
			want: Requirements{
				RequirementEqual("key", "value"),
			},
		},
		{
			name: "multiple",
			want: Requirements{
				RequirementEqual("key1", "value1"),
				Requirement{
					Key:      "key2",
					Operator: In,
					Values:   []any{"value2", "value3"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.want.String()
			got, err := ParseRequirements(expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRequirements() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchUnstructuredFieldRequirements(t *testing.T) {
	obj := &Unstructured{Object: map[string]any{
		"score": 1.5,
		"name":  "example-widget",
		"tags":  []any{"blue", "stable"},
	}}
	tests := []struct {
		name string
		req  Requirement
		want bool
	}{
		{
			name: "fractional greater than",
			req:  NewRequirement("score", GreaterThan, 1.0),
			want: true,
		},
		{
			name: "greater than or equal",
			req:  NewRequirement("score", GreaterThanOrEqual, 1.5),
			want: true,
		},
		{
			name: "less than or equal",
			req:  NewRequirement("score", LessThanOrEqual, 1.5),
			want: true,
		},
		{
			name: "slice contains",
			req:  NewRequirement("tags", Contains, "stable"),
			want: true,
		},
		{
			name: "string contains",
			req:  NewRequirement("name", Contains, "widget"),
			want: true,
		},
		{
			name: "like",
			req:  NewRequirement("name", Like, "widget"),
			want: true,
		},
		{
			name: "missing comparison value",
			req:  NewRequirement("score", GreaterThan),
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MatchUnstructuredFieldRequirments(obj, Requirements{test.req}); got != test.want {
				t.Fatalf("MatchUnstructuredFieldRequirments() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRequirementMatchLabelsMissingKey(t *testing.T) {
	if RequirementMatchLabels(RequirementEqual("missing", ""), nil) {
		t.Fatal("missing label matched equality with an empty string")
	}
	if !RequirementMatchLabels(NewRequirement("missing", NotEquals, "value"), nil) {
		t.Fatal("missing label did not match NotEquals")
	}
	if !RequirementMatchLabels(NewRequirement("missing", NotIn, "value"), nil) {
		t.Fatal("missing label did not match NotIn")
	}
}
