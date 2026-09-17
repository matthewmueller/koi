// Package koi provides typed HTTP controllers and negotiated responses.
package koi

import (
	"log/slog"
	"net/http"

	"github.com/matthewmueller/koi/internal/request"
)

// Empty represents a request or response with no data.
type Empty struct{}

// Handler chooses an outcome; Koi handles its HTTP representation.
type Handler[In, Out any] func(*Request[In]) Response[Out]

// Koi configures routes. Configure its fields before serving requests.
type Koi struct {
	// Renderer renders HTML bodies. New supplies a renderer that parses template files by name.
	// A nil Renderer causes HTML body rendering to fail with 500.
	Renderer Renderer
	// Validator checks bound input before the controller runs. New supplies
	// tag validation followed by the input’s Validate method, if present.
	Validator Validator
	// Debug exposes underlying error messages in responses. It defaults to false.
	Debug bool
	// Log records method, path, status, duration, and underlying errors.
	// New configures a discard logger. Replace it to enable request logging.
	// Successes use Info, 409 uses Warn, other 4xx responses use Debug, and
	// 5xx responses use Error.
	Log *slog.Logger
}

// New creates a dispatcher with HTML templates, validation, and a discard logger.
func New() *Koi {
	return &Koi{
		Renderer:  &htmlTemplate{},
		Validator: newValidator(),
		Log:       slog.New(slog.DiscardHandler),
	}
}

// Wrap a koi handler as an http.Handler. It unmarshals input, validates it, and
// negotiates the response.
func (k *Koi) Wrap[In, Out any](handler Handler[In, Out]) *Route[Out] {
	return &Route[Out]{k: k, handle: func(r *http.Request) (Response[Out], error) {
		req := &Request[In]{Request: r}
		if err := request.Unmarshal(r, &req.Input); err != nil {
			return Response[Out]{}, err
		}
		if invalid := k.Validator.Validate(&req.Input); invalid != nil {
			return Response[Out]{}, invalid
		}
		return handler(req), nil
	}}
}
