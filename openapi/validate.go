package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

func ValidateSchema(schema *Schema, data any) error {
	validator := NewDefaultValidator()
	output := validator.ValidateJson(*schema, data)
	if output.Valid {
		return nil
	}
	return fmt.Errorf("validation failed: %s", output.Message)
}

func ConvertToJSONCompatible(data any) (any, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var jsonCompatible any
	if err := json.Unmarshal(jsonBytes, &jsonCompatible); err != nil {
		return nil, err
	}
	return jsonCompatible, nil
}

type StringFormatValidator interface {
	Validate(ctx context.Context, schema Schema, value string) error
}

type StringFormatValidatorFunc func(ctx context.Context, schema Schema, value string) error

func (f StringFormatValidatorFunc) Validate(ctx context.Context, schema Schema, value string) error {
	return f(ctx, schema, value)
}

type ExtensionValidator interface {
	Validate(ctx context.Context, schema Schema, data any) OutPutError
}

func NewDefaultValidator() *Validator {
	return &Validator{
		StringFormats: DefaultStringFormatValidators(),
		Extensions:    map[string]ExtensionValidator{},
	}
}

type Validator struct {
	StringFormats map[string]StringFormatValidator
	Extensions    map[string]ExtensionValidator
}

type OutPut = OutPutError

type OutPutError struct {
	Valid                   bool           `json:"valid,omitempty"`
	Annotations             map[string]any `json:"annotations,omitempty"`
	KeywordLocation         string         `json:"keywordLocation,omitempty"`
	AbsoluteKeywordLocation string         `json:"absoluteKeywordLocation,omitempty"`
	InstanceLocation        string         `json:"instanceLocation,omitempty"`
	Message                 string         `json:"message,omitempty"`
	Error                   string         `json:"error,omitempty"`
	Errors                  []OutPutError  `json:"errors,omitempty"`
}

// ValidateJson validates data against the provided schema
// data must be JSON compatible: map[string]any, []any, string, float64, bool, nil
// other types not supported
func (v *Validator) ValidateJson(schema Schema, data any) OutPut {
	return v.validate(context.Background(), schema, "", data, "")
}

func (v *Validator) ValidateJsonContext(ctx context.Context, schema Schema, data any) OutPut {
	return v.validate(ctx, schema, "", data, "")
}

func (v *Validator) validate(ctx context.Context, schema Schema, keywordLocation string, data any, instanceLocation string) OutPutError {
	var outputs []OutPutError
	// if
	if schema.If != nil {
		ifOutput := v.validate(ctx, *schema.If, keywordLocation+"/if", data, instanceLocation)
		if ifOutput.Valid {
			// then
			if schema.Then != nil {
				thenOutput := v.validate(ctx, *schema.Then, keywordLocation+"/then", data, instanceLocation)
				outputs = append(outputs, thenOutput)
			}
		} else {
			// else
			if schema.Else != nil {
				elseOutput := v.validate(ctx, *schema.Else, keywordLocation+"/else", data, instanceLocation)
				outputs = append(outputs, elseOutput)
			}
		}
	}
	// allof
	if len(schema.AllOf) > 0 {
		var allofOutputs []OutPutError
		for idx, subschema := range schema.AllOf {
			subOutput := v.validate(ctx, subschema, keywordLocation+"/allOf/"+fmt.Sprint(idx), data, instanceLocation)
			allofOutputs = append(allofOutputs, subOutput)
		}
		outputs = append(outputs, aggregateAllof(allofOutputs))
	}
	// anyof
	if len(schema.AnyOf) > 0 {
		var anyofOutputs []OutPutError
		for idx, subschema := range schema.AnyOf {
			subOutput := v.validate(ctx, subschema, keywordLocation+"/anyOf/"+fmt.Sprint(idx), data, instanceLocation)
			anyofOutputs = append(anyofOutputs, subOutput)
		}
		outputs = append(outputs, aggregateAnyof(anyofOutputs))
	}
	// oneof
	if len(schema.OneOf) > 0 {
		var oneofOutputs []OutPutError
		for idx, subschema := range schema.OneOf {
			subOutput := v.validate(ctx, subschema, keywordLocation+"/oneOf/"+fmt.Sprint(idx), data, instanceLocation)
			oneofOutputs = append(oneofOutputs, subOutput)
		}
		outputs = append(outputs, aggregateOneof(oneofOutputs))
	}
	// not
	if schema.Not != nil {
		if notOutput := v.validate(ctx, *schema.Not, keywordLocation+"/not", data, instanceLocation); notOutput.Valid {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/not",
				Message:          "data must not validate against the schema in 'not'",
			})
		}
	}
	// const
	if constValue := schema.Const; constValue != nil {
		if !valueEquals(data, constValue) {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/const",
				Message:          fmt.Sprintf("object does not match const value %v", constValue),
			})
		}
	}
	// enum
	if len(schema.Enum) > 0 {
		if !slices.ContainsFunc(schema.Enum, func(enumVal any) bool { return valueEquals(data, enumVal) }) {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/enum",
				Message:          fmt.Sprintf("value %v is not in enum %v", data, schema.Enum),
			})
		}
	}
	// type
	if len(schema.Type) == 0 {
		schema.Type = StringOrArray{v.detectType(data)}
	}
	if len(schema.Type) > 0 {
		var typeOutputs []OutPutError
		for _, typ := range schema.Type {
			typeOutput := v.validateType(ctx, schema, keywordLocation, data, instanceLocation, typ)
			typeOutputs = append(typeOutputs, typeOutput)
		}
		outputs = append(outputs, aggregateAnyof(typeOutputs))
	}
	// extensions
	for extName, extValidator := range v.Extensions {
		if _, ok := schema.ExtraProps[extName]; ok {
			extOutput := extValidator.Validate(ctx, schema, data)
			outputs = append(outputs, extOutput)
		}
	}
	return aggregateAllof(outputs)
}

func (v *Validator) detectType(data any) string {
	switch data.(type) {
	case nil:
		return SchemaTypeNull
	case bool:
		return SchemaTypeBoolean
	case float64, json.Number:
		return SchemaTypeNumber
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return SchemaTypeInteger
	case string:
		return SchemaTypeString
	case []any:
		return SchemaTypeArray
	case map[string]any:
		return SchemaTypeObject
	default:
		return ""
	}
}

func asFloat64(data any) (float64, bool) {
	switch v := data.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	}
	return 0, false
}

func valueEquals(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

func (v *Validator) validateType(ctx context.Context, schema Schema, keywordLocation string, data any, instanceLocation string, typ string) OutPutError {
	switch typ {
	case SchemaTypeNumber:
		if floatval, ok := asFloat64(data); ok {
			return v.validateNumberic(ctx, schema, keywordLocation, floatval, instanceLocation)
		}
		if number, ok := data.(json.Number); ok {
			floatval, err := number.Float64()
			if err != nil {
				return OutPutError{
					InstanceLocation: instanceLocation,
					KeywordLocation:  keywordLocation + "/type",
					Message:          fmt.Sprintf("invalid number value: %v", err),
				}
			}
			return v.validateNumberic(ctx, schema, keywordLocation, floatval, instanceLocation)
		}
	case SchemaTypeInteger:
		if val, ok := asFloat64(data); ok {
			if val == float64(int64(val)) {
				return v.validateNumberic(ctx, schema, keywordLocation, val, instanceLocation)
			}
			return OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/type",
				Message:          fmt.Sprintf("expected integer value, got number %v", val),
			}
		}
		if number, ok := data.(json.Number); ok {
			floatval, err := number.Float64()
			if err != nil {
				return OutPutError{
					InstanceLocation: instanceLocation,
					KeywordLocation:  keywordLocation + "/type",
					Message:          fmt.Sprintf("invalid number value: %v", err),
				}
			}
			if floatval == float64(int64(floatval)) {
				return v.validateNumberic(ctx, schema, keywordLocation, floatval, instanceLocation)
			}
			return OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/type",
				Message:          fmt.Sprintf("expected integer value, got number %v", floatval),
			}
		}
	case SchemaTypeBoolean:
		if _, ok := data.(bool); ok {
			return OutPutError{Valid: true}
		}
	case SchemaTypeNull:
		if data == nil {
			return OutPutError{Valid: true}
		}
	case SchemaTypeArray:
		if val, ok := data.([]any); ok {
			return v.validateArray(ctx, schema, keywordLocation, val, instanceLocation)
		}
	case SchemaTypeObject:
		if val, ok := data.(map[string]any); ok {
			return v.validateObject(ctx, schema, keywordLocation, val, instanceLocation)
		}
	case SchemaTypeString:
		if strval, ok := data.(string); ok {
			return v.validateString(ctx, schema, keywordLocation, strval, instanceLocation)
		}
	case "":
		return OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/type",
			Message:          "no type specified in schema",
		}
	default:
		return OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/type",
			Message:          fmt.Sprintf("unknown type %s", typ),
		}
	}
	return OutPutError{
		InstanceLocation: instanceLocation,
		KeywordLocation:  keywordLocation + "/type",
		Message:          fmt.Sprintf("expected %s value", typ),
	}
}

func (v *Validator) validateNumberic(_ context.Context, schema Schema, keywordLocation string, data float64, instanceLocation string) OutPutError {
	var outputs []OutPutError
	// maximum
	if schema.Maximum != nil && data > *schema.Maximum {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/maximum",
			Message:          fmt.Sprintf("number %v exceeds maximum %v", data, *schema.Maximum),
		})
	}
	// exclusiveMaximum
	if schema.ExclusiveMaximum != nil && data >= *schema.ExclusiveMaximum {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/exclusiveMaximum",
			Message:          fmt.Sprintf("number %v exceeds or equals exclusiveMaximum %v", data, *schema.ExclusiveMaximum),
		})
	}
	// minimum
	if schema.Minimum != nil && (data < *schema.Minimum) {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/minimum",
			Message:          fmt.Sprintf("number %v is less than minimum %v", data, *schema.Minimum),
		})
	}
	// exclusiveMinimum
	if schema.ExclusiveMinimum != nil && data <= *schema.ExclusiveMinimum {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/exclusiveMinimum",
			Message:          fmt.Sprintf("number %v is less than or equals exclusiveMinimum %v", data, *schema.ExclusiveMinimum),
		})
	}
	// multipleOf
	if schema.MultipleOf != nil {
		if *schema.MultipleOf == 0 {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/multipleOf",
				Message:          "multipleOf cannot be zero",
			})
		} else if remainder := data / *schema.MultipleOf; remainder != float64(int64(remainder)) {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/multipleOf",
				Message:          fmt.Sprintf("number %v is not a multiple of %v", data, *schema.MultipleOf),
			})
		}
	}
	return aggregateAllof(outputs)
}

func (v *Validator) validateString(ctx context.Context, schema Schema, keywordLocation string, data string, instanceLocation string) OutPutError {
	var outputs []OutPutError
	// maxLength
	if schema.MaxLength != nil && int64(utf8.RuneCountInString(data)) > *schema.MaxLength {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/maxLength",
			Message:          fmt.Sprintf("string length %d exceeds maxLength %d", utf8.RuneCountInString(data), *schema.MaxLength),
		})
	}
	// minLength
	if schema.MinLength != nil && int64(utf8.RuneCountInString(data)) < *schema.MinLength {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/minLength",
			Message:          fmt.Sprintf("string length %d is less than minLength %d", utf8.RuneCountInString(data), *schema.MinLength),
		})
	}
	// pattern
	if schema.Pattern != "" {
		regexp, err := regexp.Compile(schema.Pattern)
		if err != nil {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/pattern",
				Message:          fmt.Sprintf("invalid pattern %s: %v", schema.Pattern, err),
			})
		} else if !regexp.MatchString(data) {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/pattern",
				Message:          fmt.Sprintf("string %s does not match pattern %s", data, schema.Pattern),
			})
		}
	}
	// format
	if schema.Format != "" {
		if formatValidator, ok := v.StringFormats[schema.Format]; ok {
			if err := formatValidator.Validate(ctx, schema, data); err != nil {
				outputs = append(outputs, OutPutError{
					InstanceLocation: instanceLocation,
					KeywordLocation:  keywordLocation + "/format",
					Message:          fmt.Sprintf("string %s does not match format %s: %v", data, schema.Format, err),
				})
			}
		}
	}
	return aggregateAllof(outputs)
}

func (v *Validator) validateArray(ctx context.Context, schema Schema, keywordLocation string, data []any, instanceLocation string) OutPutError {
	var outputs []OutPutError
	// maxItems
	if schema.MaxItems != nil && int64(len(data)) > *schema.MaxItems {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/maxItems",
			Message:          fmt.Sprintf("array has %d items, exceeds maxItems %d", len(data), *schema.MaxItems),
		})
	}
	// minItems
	if schema.MinItems != nil && int64(len(data)) < *schema.MinItems {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/minItems",
			Message:          fmt.Sprintf("array has %d items, less than minItems %d", len(data), *schema.MinItems),
		})
	}
	// uniqueItems
	if schema.UniqueItems {
		for idx, item := range data {
			firstidx := slices.IndexFunc(data, func(other any) bool { return valueEquals(item, other) })
			if firstidx != idx {
				outputs = append(outputs, OutPutError{
					InstanceLocation: instanceLocation,
					KeywordLocation:  keywordLocation + "/uniqueItems",
					Message:          fmt.Sprintf("array items are not unique, item at index %d is a duplicate of item at index %d", idx, firstidx),
				})
			}
		}
	}
	// prefixItems
	if len(schema.PrefixItems) > 0 {
		for idx, itemSchema := range schema.PrefixItems {
			if idx >= len(data) {
				break
			}
			itemOutput := v.validate(ctx, itemSchema, keywordLocation+"/prefixItems/"+fmt.Sprintf("%d", idx), data[idx], fmt.Sprintf("%s/%d", instanceLocation, idx))
			if !itemOutput.Valid {
				outputs = append(outputs, itemOutput)
			}
		}
	}
	// items
	// Note that the behavior of "items" without "prefixItems" is identical to that of the schema form of "items" in prior drafts. When "prefixItems" is present, the behavior of "items" is identical to the former "additionalItems" keyword.
	if schema.Items != nil {
		for idx, item := range data {
			if len(schema.PrefixItems) > 0 && idx < len(schema.PrefixItems) {
				continue
			}
			itemOutput := v.validate(ctx, *schema.Items, keywordLocation+"/items", item, fmt.Sprintf("%s/%d", instanceLocation, idx))
			if !itemOutput.Valid {
				outputs = append(outputs, itemOutput)
			}
		}
	}
	// contains
	if schema.Contains != nil {
		// contains
		containsCount := int64(0)
		for idx, item := range data {
			itemOutput := v.validate(ctx, *schema.Contains, keywordLocation+"/contains", item, fmt.Sprintf("%s/%d", instanceLocation, idx))
			if itemOutput.Valid {
				containsCount++
			}
		}
		// maxContains
		if schema.MaxContains != nil && containsCount > *schema.MaxContains {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/maxContains",
				Message:          fmt.Sprintf("array contains too many items matching 'contains' schema, maximum allowed is %d", *schema.MaxContains),
			})
		}
		// minContains
		if schema.MinContains != nil && containsCount < *schema.MinContains {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/minContains",
				Message:          fmt.Sprintf("array does not contain enough items matching 'contains' schema, need %d but got %d", *schema.MinContains, containsCount),
			})
		}
		if schema.MinContains == nil && containsCount == 0 {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/contains",
				Message:          "array does not contain any items matching 'contains' schema",
			})
		}
	}
	return aggregateAllof(outputs)
}

func (v *Validator) validateObject(ctx context.Context, schema Schema, keywordLocation string, data map[string]any, instanceLocation string) OutPutError {
	var outputs []OutPutError
	// maxProperties
	if schema.MaxProperties != nil && int64(len(data)) > *schema.MaxProperties {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/maxProperties",
			Message:          fmt.Sprintf("object has %d properties, exceeds maxProperties %d", len(data), *schema.MaxProperties),
		})
	}
	// minProperties
	if schema.MinProperties != nil && int64(len(data)) < *schema.MinProperties {
		outputs = append(outputs, OutPutError{
			InstanceLocation: instanceLocation,
			KeywordLocation:  keywordLocation + "/minProperties",
			Message:          fmt.Sprintf("object has %d properties, less than minProperties %d", len(data), *schema.MinProperties),
		})
	}
	// required
	for _, reqProp := range schema.Required {
		if _, ok := data[reqProp]; !ok {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/required",
				Message:          fmt.Sprintf("missing required property %s", reqProp),
			})
		}
	}
	// dependentrequired
	for depKey, depProps := range schema.DependentRequired {
		if _, ok := data[depKey]; ok {
			for _, depProp := range depProps {
				if _, ok := data[depProp]; !ok {
					outputs = append(outputs, OutPutError{
						InstanceLocation: instanceLocation,
						KeywordLocation:  keywordLocation + "/dependentRequired" + "/" + jsonPointerEscape(depKey),
						Message:          fmt.Sprintf("property %s is required when %s is present", depProp, depKey),
					})
				}
			}
		}
	}

	validatedKeys := map[string]struct{}{}
	// properties
	for _, prop := range schema.Properties {
		propName, propSchema := prop.Name, prop.Schema
		// record validated keys
		validatedKeys[propName] = struct{}{}
		propValue := data[propName]
		propOutput := v.validate(ctx, propSchema, jsonPointerJoin(keywordLocation, propName), propValue, jsonPointerJoin(instanceLocation, propName))
		if !propOutput.Valid {
			outputs = append(outputs, propOutput)
		}
	}
	// patternProperties
	for _, prop := range schema.PatternProperties {
		pattern, propSchema := prop.Name, prop.Schema
		regexp, err := regexp.Compile(pattern)
		if err != nil {
			outputs = append(outputs, OutPutError{
				InstanceLocation: instanceLocation,
				KeywordLocation:  keywordLocation + "/patternProperties/" + jsonPointerEscape(pattern),
				Message:          fmt.Sprintf("invalid pattern: %s", err.Error()),
			})
			continue
		}
		for dataKey, dataValue := range data {
			if regexp.MatchString(dataKey) {
				// record validated keys
				validatedKeys[dataKey] = struct{}{}
				propOutput := v.validate(ctx, propSchema, jsonPointerJoin(keywordLocation+"/patternProperties", pattern), dataValue, jsonPointerJoin(instanceLocation, dataKey))
				if !propOutput.Valid {
					outputs = append(outputs, propOutput)
				}
			}
		}
	}
	// additionalProperties
	if schema.AdditionalProperties != nil {
		for dataKey, dataValue := range data {
			// already validated
			if _, validated := validatedKeys[dataKey]; validated {
				continue
			}
			if !schema.AdditionalProperties.Allows {
				outputs = append(outputs, OutPutError{
					InstanceLocation: instanceLocation,
					KeywordLocation:  keywordLocation + "/additionalProperties",
					Message:          fmt.Sprintf("additional property %s is not allowed", dataKey),
				})
				continue
			}
			if schema.AdditionalProperties.Schema != nil {
				propOutput := v.validate(ctx, *schema.AdditionalProperties.Schema, jsonPointerJoin(keywordLocation, "additionalProperties"), dataValue, jsonPointerJoin(instanceLocation, dataKey))
				outputs = append(outputs, propOutput)
			}
		}
	}
	// propertyNames
	if schema.PropertyNames != nil {
		for dataKey := range data {
			propOutput := v.validate(ctx, *schema.PropertyNames, jsonPointerJoin(keywordLocation, "propertyNames"), dataKey, jsonPointerJoin(instanceLocation, dataKey))
			outputs = append(outputs, propOutput)
		}
	}
	// dependentSchemas
	for depKey, depSchema := range schema.DependentSchemas {
		if _, ok := data[depKey]; ok {
			schemaOutput := v.validate(ctx, depSchema, jsonPointerJoin(keywordLocation, "dependentSchemas/"+jsonPointerEscape(depKey)), data, instanceLocation)
			outputs = append(outputs, schemaOutput)
		}
	}
	return aggregateAllof(outputs)
}

func DefaultStringFormatValidators() map[string]StringFormatValidator {
	return map[string]StringFormatValidator{
		"date-time":             StringFormatValidatorFunc(validateDateTime),
		"date":                  StringFormatValidatorFunc(validateDate),
		"email":                 StringFormatValidatorFunc(validateEmail),
		"hostname":              StringFormatValidatorFunc(validateHostname),
		"ipv4":                  StringFormatValidatorFunc(validateIPv4),
		"ipv6":                  StringFormatValidatorFunc(validateIPv6),
		"uri":                   StringFormatValidatorFunc(validateURI),
		"uuid":                  StringFormatValidatorFunc(validateUUID),
		"duration":              StringFormatValidatorFunc(validateDuration),
		"regex":                 StringFormatValidatorFunc(validateRegex),
		"json-pointer":          StringFormatValidatorFunc(validateJSONPointer),
		"relative-json-pointer": StringFormatValidatorFunc(validateRelativeJSONPointer),
	}
}

func validateDateTime(ctx context.Context, schema Schema, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("invalid date-time format")
	}
	return nil
}

func validateDate(ctx context.Context, schema Schema, value string) error {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return fmt.Errorf("invalid date format")
	}
	return nil
}

func validateEmail(ctx context.Context, schema Schema, value string) error {
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

func validateHostname(ctx context.Context, schema Schema, value string) error {
	if len(value) > 255 {
		return fmt.Errorf("invalid hostname format")
	}
	matched, _ := regexp.MatchString(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`, value)
	if !matched {
		return fmt.Errorf("invalid hostname format")
	}
	return nil
}

func validateIPv4(ctx context.Context, schema Schema, value string) error {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("invalid ipv4 format")
	}
	return nil
}

func validateIPv6(ctx context.Context, schema Schema, value string) error {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() != nil || !strings.Contains(value, ":") {
		return fmt.Errorf("invalid ipv6 format")
	}
	return nil
}

func validateURI(ctx context.Context, schema Schema, value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("invalid uri format")
	}
	return nil
}

func validateUUID(ctx context.Context, schema Schema, value string) error {
	matched, _ := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, value)
	if !matched {
		return fmt.Errorf("invalid uuid format")
	}
	return nil
}

func validateDuration(ctx context.Context, schema Schema, value string) error {
	matched, _ := regexp.MatchString(`^P(\d+Y)?(\d+M)?(\d+W)?(\d+D)?(T(\d+H)?(\d+M)?(\d+S)?)?$`, value)
	// The original regex used a negative lookahead to prevent matching just "P".
	// In Go, we check this explicitly:
	if !matched || value == "P" {
		return fmt.Errorf("invalid duration format")
	}
	return nil
}

func validateRegex(ctx context.Context, schema Schema, value string) error {
	if _, err := regexp.Compile(value); err != nil {
		return fmt.Errorf("invalid regex format")
	}
	return nil
}

func validateJSONPointer(ctx context.Context, schema Schema, value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("invalid json-pointer format")
	}
	for i := 0; i < len(value); i++ {
		if value[i] == '~' {
			if i+1 >= len(value) {
				return fmt.Errorf("invalid json-pointer format")
			}
			c := value[i+1]
			if c != '0' && c != '1' {
				return fmt.Errorf("invalid json-pointer format")
			}
			i++
		}
	}
	return nil
}

func validateRelativeJSONPointer(ctx context.Context, schema Schema, value string) error {
	if value == "" {
		return fmt.Errorf("invalid relative-json-pointer format")
	}
	i := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
	}
	if i == 0 {
		return fmt.Errorf("invalid relative-json-pointer format")
	}
	if value[0] == '0' && i > 1 {
		return fmt.Errorf("invalid relative-json-pointer format")
	}
	if i == len(value) {
		return nil
	}
	suffix := value[i:]
	if suffix == "#" {
		return nil
	}
	if strings.HasPrefix(suffix, "/") {
		for j := 0; j < len(suffix); j++ {
			if suffix[j] == '~' {
				if j+1 >= len(suffix) {
					return fmt.Errorf("invalid relative-json-pointer format")
				}
				c := suffix[j+1]
				if c != '0' && c != '1' {
					return fmt.Errorf("invalid relative-json-pointer format")
				}
				j++
			}
		}
		return nil
	}
	return fmt.Errorf("invalid relative-json-pointer format")
}

// aggregateAllof aggregates multiple OutPutError
// it returns a valid OutPutError only if all of the outputs are valid
func aggregateAllof(outputs []OutPutError) OutPutError {
	aggregated := OutPutError{Valid: true}
	for _, output := range outputs {
		if !output.Valid {
			aggregated.Valid = false
			aggregated.Errors = append(aggregated.Errors, output)
		}
	}
	return aggregated
}

// aggregateAnyof aggregates multiple OutPutError
// it returns a valid OutPutError if any of the outputs is valid
func aggregateAnyof(outputs []OutPutError) OutPutError {
	aggregated := OutPutError{Valid: false}
	for _, output := range outputs {
		if output.Valid {
			return OutPutError{Valid: true}
		}
		aggregated.Errors = append(aggregated.Errors, output)
	}
	return aggregated
}

func aggregateOneof(outputs []OutPutError) OutPutError {
	validCount := 0
	var aggregated OutPutError
	for _, output := range outputs {
		if output.Valid {
			validCount++
		} else {
			aggregated.Errors = append(aggregated.Errors, output)
		}
	}
	if validCount == 1 {
		return OutPutError{Valid: true}
	}
	aggregated.Valid = false
	if validCount == 0 {
		aggregated.Message = "no subschema in oneOf matched"
	} else {
		aggregated.Message = fmt.Sprintf("%d subschemas in oneOf matched", validCount)
	}
	return aggregated
}

func jsonPointerJoin(base, token string) string {
	if base == "" || base == "/" {
		return "/" + jsonPointerEscape(token)
	}
	return base + "/" + jsonPointerEscape(token)
}

func jsonPointerEscape(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

func jsonPointerUnescape(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}
