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
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		for _, tag := range []string{"json", "form"} {
			name, _, _ := strings.Cut(field.Tag.Get(tag), ",")
			if name != "" && name != "-" {
				return name
			}
		}
		return field.Name
	})
	return &inputValidator{validator: v}
}

func (v *inputValidator) Validate(input any) Invalid {
	value := reflect.ValueOf(input)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		// A nil pointer represents absent input and cannot be dereferenced.
		if value.IsNil() {
			return nil
		}
		// Check before dereferencing so pointer receiver validators are found.
		if custom, ok := value.Interface().(interface{ Validate() Invalid }); ok {
			return custom.Validate()
		}
		// Follow pointers until the underlying value can be inspected as a struct.
		value = value.Elem()
	}
	// nil input and pointers that resolve to nil have nothing to validate.
	if !value.IsValid() {
		return nil
	}

	if value.Kind() == reflect.Struct {
		// Struct unwraps only one pointer; use the struct resolved above for nested pointer inputs.
		if err := v.validator.Struct(value.Interface()); err != nil {
			fields := err.(validator.ValidationErrors)
			var result Invalid
			for _, field := range fields {
				name := field.Namespace()
				if value.Type().Name() != "" {
					_, name, _ = strings.Cut(name, ".")
				}
				result.Add(name, validationMessage(field.Tag()))
			}
			return result
		}
	}
	return nil
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
