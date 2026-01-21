package validation

import (
	"context"
	"testing"
)

func TestValidator_Validate(t *testing.T) {
	type Address struct {
		City   string `json:"city" validation:"required,len 1 50"`
		Street string `json:"street" validation:"required"`
	}

	type User struct {
		Name    string   `json:"name" validation:"required,len 1 20"`
		Age     int      `json:"age" validation:"range 0 150"`
		Email   string   `json:"email" validation:"regexp ^[a-z]+@.*"`
		Tags    []string `json:"tags" validation:"len 0 5"`
		Address Address  `json:"address"`
	}

	tests := []struct {
		name       string
		data       User
		wantErrors int
		wantRules  []string
	}{
		{
			name: "valid user",
			data: User{
				Name:  "John",
				Age:   25,
				Email: "john@example.com",
				Tags:  []string{"dev"},
				Address: Address{
					City:   "New York",
					Street: "123 Main St",
				},
			},
			wantErrors: 0,
		},
		{
			name: "empty name",
			data: User{
				Name:  "",
				Age:   25,
				Email: "test@example.com",
				Address: Address{
					City:   "NYC",
					Street: "Main",
				},
			},
			wantErrors: 1,
			wantRules:  []string{"len"},
		},
		{
			name: "age out of range",
			data: User{
				Name:  "John",
				Age:   200,
				Email: "test@example.com",
				Address: Address{
					City:   "NYC",
					Street: "Main",
				},
			},
			wantErrors: 1,
			wantRules:  []string{"range"},
		},
		{
			name: "multiple errors",
			data: User{
				Name:  "",
				Age:   -5,
				Email: "invalid",
				Address: Address{
					City:   "",
					Street: "",
				},
			},
			wantErrors: 4, // len + range + len(city) + required(street)
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.Validate(context.Background(), &tt.data)

			if tt.wantErrors == 0 {
				if errs != nil && errs.HasErrors() {
					t.Errorf("expected no errors, got %d: %v", len(errs.Errors), errs)
				}
				return
			}

			if errs == nil {
				t.Errorf("expected %d errors, got nil", tt.wantErrors)
				return
			}

			if len(errs.Errors) != tt.wantErrors {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrors, len(errs.Errors), errs)
			}

			// Check rules if specified
			for _, rule := range tt.wantRules {
				found := false
				for _, err := range errs.Errors {
					if err.Rule == rule {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected rule %s not found", rule)
				}
			}
		})
	}
}

func TestValidator_JSONPointer(t *testing.T) {
	type Container struct {
		Name string `json:"name" validation:"required"`
	}
	type Spec struct {
		Containers []Container `json:"containers"`
	}
	type Pod struct {
		Spec Spec `json:"spec"`
	}

	validator := NewValidator()
	pod := Pod{
		Spec: Spec{
			Containers: []Container{
				{Name: ""},
			},
		},
	}

	errs := validator.Validate(context.Background(), &pod)
	if errs == nil || len(errs.Errors) == 0 {
		t.Fatal("expected errors")
	}

	err := errs.Errors[0]
	expectedPath := "/spec/containers/0/name"

	if err.Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, err.Path)
	}
}

func TestValidator_DecodeAndValidate(t *testing.T) {
	type User struct {
		Name string `json:"name" validation:"required,len 1 10"`
		Age  int    `json:"age" validation:"range 0 150"`
	}

	validator := NewValidator()

	// Test valid JSON
	validJSON := []byte(`{"name": "John", "age": 25}`)
	var user User
	errs := validator.DecodeAndValidate(context.Background(), validJSON, &user)
	if errs != nil {
		t.Errorf("unexpected errors for valid JSON: %v", errs)
	}

	// Test invalid type
	invalidTypeJSON := []byte(`{"name": "John", "age": "twenty"}`)
	var user2 User
	errs = validator.DecodeAndValidate(context.Background(), invalidTypeJSON, &user2)
	if errs == nil {
		t.Error("expected error for invalid type")
	} else if errs.Errors[0].Rule != "type" {
		t.Errorf("expected rule 'type', got %s", errs.Errors[0].Rule)
	}

	// Test validation error
	invalidValueJSON := []byte(`{"name": "", "age": 200}`)
	var user3 User
	errs = validator.DecodeAndValidate(context.Background(), invalidValueJSON, &user3)
	if errs == nil {
		t.Error("expected validation errors")
	}
}

func TestFieldPath(t *testing.T) {
	tests := []struct {
		path    *FieldPath
		wantDot string
		wantPtr string
	}{
		{
			path:    NewFieldPath().AppendField("spec").AppendField("containers").AppendIndex(0).AppendField("name"),
			wantDot: "spec.containers[0].name",
			wantPtr: "/spec/containers/0/name",
		},
		{
			path:    NewFieldPath().AppendField("metadata").AppendField("labels").AppendKey("app"),
			wantDot: "metadata.labels[app]",
			wantPtr: "/metadata/labels/app",
		},
		{
			path:    NewFieldPath(),
			wantDot: "",
			wantPtr: "",
		},
	}

	for _, tt := range tests {
		gotDot := tt.path.DotNotation()
		gotPtr := tt.path.JSONPointer()

		if gotDot != tt.wantDot {
			t.Errorf("DotNotation: got %s, want %s", gotDot, tt.wantDot)
		}
		if gotPtr != tt.wantPtr {
			t.Errorf("JSONPointer: got %s, want %s", gotPtr, tt.wantPtr)
		}
	}
}

func TestRules(t *testing.T) {
	tests := []struct {
		name    string
		rule    string
		value   interface{}
		params  []string
		wantErr bool
	}{
		// required
		{"required nil", "required", nil, nil, true},
		{"required value", "required", "hello", nil, false},

		// notempty
		{"notempty empty string", "notempty", "", nil, true},
		{"notempty with value", "notempty", "hello", nil, false},
		{"notempty empty slice", "notempty", []string{}, nil, true},

		// len
		{"len exact match", "len", "abc", []string{"3"}, false},
		{"len exact mismatch", "len", "ab", []string{"3"}, true},
		{"len range valid", "len", "hello", []string{"1", "10"}, false},
		{"len range invalid", "len", "hello world", []string{"1", "5"}, true},

		// range
		{"range valid", "range", 50, []string{"0", "100"}, false},
		{"range invalid", "range", 150, []string{"0", "100"}, true},

		// regexp
		{"regexp match", "regexp", "hello", []string{"^[a-z]+$"}, false},
		{"regexp no match", "regexp", "Hello123", []string{"^[a-z]+$"}, true},

		// in
		{"in valid", "in", "a", []string{"a", "b", "c"}, false},
		{"in invalid", "in", "d", []string{"a", "b", "c"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok := DefaultRules[tt.rule]
			if !ok {
				t.Fatalf("rule %s not found", tt.rule)
			}

			err := rule(context.Background(), tt.value, tt.params...)

			if tt.wantErr && err == nil {
				t.Errorf("expected error but got valid")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected valid but got error: %v", err)
			}
		})
	}
}
