package koi

import (
	"context"

	"github.com/go-playground/validator/v10"
)

var (
	valid = validator.New()
)

// Validate the input struct.
// TODO: support an override to customize validation.
var validate = func(ctx context.Context, input any) error {
	return valid.StructCtx(ctx, input)
}

// Customize the status code returned from an error.
type statuser interface {
	Status() int
}
