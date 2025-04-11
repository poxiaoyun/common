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
