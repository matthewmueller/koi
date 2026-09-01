package koi

import (
	"context"
	"log/slog"
	"net/http"
)

// Validator is an interface for validating input.
type Validator interface {
	Validate() error
}

// Statuser is an interface for customizing the status code of an error.
type Statuser interface {
	Status() int
}

func New(log *slog.Logger) *Koi {
	return &Koi{log}
}

type Koi struct {
	log *slog.Logger
}

func (k *Koi) Create[In, Out any](func(ctx context.Context, in In) (out Out, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	})
}
