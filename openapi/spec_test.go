package openapi

import (
	"encoding/json"
	"testing"
)

func TestSchema_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    any
		wantErr bool
	}{
		{
			data: `{"type":"object","properties":{"property1":{"type":"string"},"property0":{"type":"integer"}}}`,
			name: "valid object",
			want: &Schema{
				SchemaProps: SchemaProps{
					Properties: SchemaProperties{
						{
							Key:   "property1",
							Value: Schema{SchemaProps: SchemaProps{Type: []string{"string"}}},
						},
						{
							Key:   "property0",
							Value: Schema{SchemaProps: SchemaProps{Type: []string{"integer"}}},
						},
					},
				},
			},
		},
		{
			data: `{"type":"object","properties":{"sku":{"type":"string","x-sku-enum": {}}}}`,
			name: "valid object with x-sku-enum extension but empty config",
			want: &Schema{
				SchemaProps: SchemaProps{
					Properties: SchemaProperties{
						{
							Key: "sku",
							Value: Schema{
								SchemaProps: SchemaProps{Type: []string{"string"}},
								Extensions: map[string]any{
									"x-sku-enum": map[string]any{},
								},
							},
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

func TestSchema_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		data Schema
		want string
	}{
		{
			name: "valid object with x-sku-enum extension but empty config",
			data: Schema{
				SchemaProps: SchemaProps{
					Type: []string{"object"},
					Properties: SchemaProperties{
						{
							Key: "sku",
							Value: Schema{
								SchemaProps: SchemaProps{Type: []string{"string"}},
								Extensions: map[string]any{
									"x-sku-enum": map[string]any{},
								},
							},
						},
					},
				},
			},
			want: `{"type":"object","properties":{"sku":{"type":"string","x-sku-enum":{}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.data)
			if err != nil {
				t.Errorf("SchemaProperties.MarshalJSON() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("SchemaProperties.MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}
