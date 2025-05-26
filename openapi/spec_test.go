package openapi

import (
	"encoding/json"
	"testing"
)

func TestSchemaProperties_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    any
		wantErr bool
	}{
		{
			data: `{"type":"object","properties":{"property1":{"type":"string"},"property2":{"type":"integer"}}}`,
			name: "valid object",
			want: &Schema{
				SchemaProps: SchemaProps{
					Properties: SchemaProperties{
						{
							Name:   "property1",
							Schema: Schema{SchemaProps: SchemaProps{Type: []string{"string"}}},
						},
						{
							Name:   "property2",
							Schema: Schema{SchemaProps: SchemaProps{Type: []string{"integer"}}},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tt.data), tt.want); (err != nil) != tt.wantErr {
				t.Errorf("SchemaProperties.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
