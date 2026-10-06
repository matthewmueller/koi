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
	// Validator checks bound input before the controller runs and response
	// bodies before they are sent. New supplies tag validation and the
	// value’s Validate method, if present, and merges their field errors.
	Validator Validator
	// MaxBodySize limits the request body bytes read while binding input.
	// Larger bodies are rejected with 413. New sets it to 10MB; zero or
	// negative disables the limit.
	MaxBodySize int64
	// Debug exposes underlying error messages in responses. It defaults to false.
	Debug bool
	// Log records method, path, status, duration, and underlying errors.
	// New configures a discard logger. Replace it to enable request logging.
	// Successes use Info, 409 uses Warn, other 4xx responses use Debug, and
	// 5xx responses use Error.
	Log *slog.Logger
}

// defaultMaxBodySize is the request body limit configured by New.
const defaultMaxBodySize = 10 << 20

// New creates a dispatcher with HTML templates, validation, a 10MB body limit,
// and a discard logger.
func New() *Koi {
	return &Koi{
		Renderer:    &htmlTemplate{},
		Validator:   newValidator(),
		MaxBodySize: defaultMaxBodySize,
		Log:         slog.New(slog.DiscardHandler),
	}
}

// Wrap a koi handler as an http.Handler. It unmarshals input, validates it, and
// negotiates the response.
func (k *Koi) Wrap[In, Out any](handler Handler[In, Out]) *Route[Out] {
	return &Route[Out]{k: k, handle: func(w http.ResponseWriter, r *http.Request) (Response[Out], error) {
		req := &Request[In]{Request: r}

		// Skip reading and validating if the input is empty
		if _, ok := any(req.Input).(Empty); ok {
			return handler(req), nil
		}

		// Read and validate the input
		if k.MaxBodySize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, k.MaxBodySize)
		}
		if err := request.Unmarshal(r, &req.Input); err != nil {
			return Response[Out]{}, err
		}
		if invalid := k.Validator.Validate(&req.Input); invalid != nil {
			return Response[Out]{}, invalid
		}

		return handler(req), nil
	}}
}
