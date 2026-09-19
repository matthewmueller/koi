package koi_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/koi"
)

func TestValidationPointerInput(t *testing.T) {
	is := is.New(t)
	type input struct {
		Email string `json:"email" validate:"required,email"`
	}
	called := false
	route := koi.New().Wrap(func(req *koi.Request[*input]) (res koi.Response[koi.Empty]) {
		called = true
		is.Equal(req.Input.Email, "matt@example.com")
		return res
	})
	req := httptest.NewRequest("POST", "/users", strings.NewReader(`{"email":"matt@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 204)
	is.True(called)

	called = false
	req = httptest.NewRequest("POST", "/users", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 422)
	is.True(!called)
	is.Equal(w.Body.String(), "{\"error\":\"Email is required\",\"fields\":{\"email\":[\"is required\"]}}\n")
}

func TestValidationFields(t *testing.T) {
	is := is.New(t)
	type address struct {
		Email string `json:"email" validate:"required,email"`
	}
	type input struct {
		Name    string  `form:"name" validate:"required"`
		Address address `json:"address"`
	}
	err := koi.New().Validator.Validate(&input{})
	is.True(err != nil)
	is.Equal(err["name"], []string{"is required"})
	is.Equal(err["address.email"], []string{"is required"})
}

func TestInvalidAdd(t *testing.T) {
	is := is.New(t)
	var err koi.Invalid
	err.Add("email", "is required")
	err.Add("email", "must be unique")
	is.Equal(err["email"], []string{"is required", "must be unique"})
	is.Equal(err.Error(), "Email is required and must be unique")
}

func TestInvalidError(t *testing.T) {
	is := is.New(t)
	invalid := koi.Invalid{
		"name":  {"is required"},
		"email": {"is required", "must be unique"},
	}
	is.Equal(invalid.Error(), "Email is required and must be unique. Name is required")
	invalid["email"] = append(invalid["email"], "must contain more than 3 characters")
	is.Equal(invalid.Error(), "Email is required, must be unique, and must contain more than 3 characters. Name is required")
}

func TestValidationAnonymous(t *testing.T) {
	is := is.New(t)
	err := koi.New().Validator.Validate(&struct {
		Name string `json:"name" validate:"required"`
	}{})
	is.True(err != nil)
	is.Equal(err["name"], []string{"is required"})
}

func TestValidationScalars(t *testing.T) {
	is := is.New(t)
	validator := koi.New().Validator
	err := validator.Validate("hello")
	is.True(err == nil)
	err = validator.Validate([]byte("hello"))
	is.True(err == nil)
	err = validator.Validate(nil)
	is.True(err == nil)
}

type customValidation struct {
	err koi.Invalid
}

func (v *customValidation) Validate() koi.Invalid { return v.err }

func TestValidationCustom(t *testing.T) {
	is := is.New(t)
	validator := koi.New().Validator
	var invalid koi.Invalid
	invalid.Add("name", "is already taken")
	err := validator.Validate(&customValidation{err: invalid})
	is.Equal(err["name"], []string{"is already taken"})
	err = validator.Validate(&customValidation{})
	is.True(err == nil)
}

type customValidator struct{}

var _ koi.Validator = (*customValidator)(nil)

func (v *customValidator) Validate(value any) koi.Invalid {
	input, ok := value.(*customValidatorInput)
	if !ok || input.Name != "matt" {
		return nil
	}
	var invalid koi.Invalid
	invalid.Add("name", "is already taken")
	return invalid
}

type customValidatorInput struct {
	Name string `json:"name"`
}

func TestValidatorOverride(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Validator = &customValidator{}
	called := 0
	route := k.Wrap(func(req *koi.Request[customValidatorInput]) (res koi.Response[koi.Empty]) {
		called++
		is.Equal(req.Input.Name, "jane")
		return res
	})
	req := httptest.NewRequest("POST", "/users", strings.NewReader(`{"name":"jane"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 204)
	is.Equal(called, 1)

	req = httptest.NewRequest("POST", "/users", strings.NewReader(`{"name":"matt"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 422)
	is.Equal(called, 1)
	is.Equal(w.Body.String(), "{\"error\":\"Name is already taken\",\"fields\":{\"name\":[\"is already taken\"]}}\n")
}
