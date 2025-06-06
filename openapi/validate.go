package openapi

import (
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

func ValidateSchema(schema *Schema, data any) error {
	if schema == nil {
		return nil // No schema to validate against
	}
	specshecma := ConvertSchemaToSpecSchema(*schema)
	return validate.AgainstSchema(&specshecma, data, strfmt.Default)
}
