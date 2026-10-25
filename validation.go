package koi

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator checks bound request input for client-facing field errors.
type Validator interface {
	// Validate receives a pointer to the bound input, which may itself be a
	// pointer. Return nil to allow the controller to run.
	Validate(any) Invalid
}

type inputValidator struct {
	validator *validator.Validate
}

var _ Validator = (*inputValidator)(nil)

func newValidator() Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	// An empty name falls back to the field name.
	v.RegisterTagNameFunc(fieldName)
	return &inputValidator{validator: v}
}

func (v *inputValidator) Validate(input any) Invalid {
	value := reflect.ValueOf(input)
	// Tag errors come first, followed by the input's own Validate errors.
	result := v.validateTags(value)
	for field, messages := range validateCustom(value) {
		for _, message := range messages {
			result.Add(field, message)
		}
	}
	return result
}

// validateTags follows pointers to a struct and checks its validate tags.
func (v *inputValidator) validateTags(value reflect.Value) Invalid {
	for value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}
	// Struct unwraps only one pointer, so pass the resolved struct.
	err := v.validator.Struct(value.Interface())
	if err == nil {
		return nil
	}
	var result Invalid
	for _, field := range err.(validator.ValidationErrors) {
		name := field.Namespace()
		if value.Type().Name() != "" {
			_, name, _ = strings.Cut(name, ".")
		}
		result.Add(name, validationMessage(field.Tag()))
	}
	return result
}

// validateCustom calls the first Validate method found while following
// pointers. Checking before dereferencing finds pointer receiver methods.
func validateCustom(value reflect.Value) Invalid {
	for value.Kind() == reflect.Pointer && !value.IsNil() {
		if custom, ok := reflect.TypeAssert[interface{ Validate() Invalid }](value); ok {
			return custom.Validate()
		}
		value = value.Elem()
	}
	return nil
}

// fieldName resolves a field's name the same way as binding in
// internal/request, so errors match the submitted keys: the form tag, then the
// json tag, then the field name. Returns an empty string for ignored fields.
func fieldName(field reflect.StructField) string {
	for _, key := range []string{"form", "json"} {
		tag, ok := field.Tag.Lookup(key)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
	}
	return field.Name
}

func validationMessage(tag string) string {
	switch tag {
	case "required", "required_if", "required_unless", "required_with", "required_with_all", "required_without", "required_without_all":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url", "http_url", "uri":
		return "must be a valid URL"
	default:
		return "is invalid"
	}
}
