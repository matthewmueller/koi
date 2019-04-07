package koi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/golang/gddo/httputil/header"
)

var (
	// errorInterface is the error interface.
	errorInterface = reflect.TypeOf((*error)(nil)).Elem()

	// contextInterface is the context interface.
	contextInterface = reflect.TypeOf((*context.Context)(nil)).Elem()

	// unmarshaler interface is the json.Unmarshaler interface.
	unmarshalerInterface = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

	// offers
	defaultAccepts = []string{"application/json", "text/html", "text/plain"}
)

// New koi with optional middleware
func New(middleware ...func(next http.Handler) http.Handler) *Koi {
	return &Koi{
		Middleware: compose(middleware...),
		Read:       ioutil.ReadAll,
		Accepts:    defaultAccepts,
		Write:      defaultWrite,
	}
}

// Koi struct
type Koi struct {
	Middleware func(next http.Handler) http.Handler
	Read       func(r io.Reader) ([]byte, error)
	Write      func(w http.ResponseWriter, accepts string, v interface{}) error
	Accepts    []string
}

type input string

// inputs
const (
	responseInput input = "http.ResponseWriter"
	contextInput  input = "context.Context"
	requestInput  input = "*http.Request"
	bodyInput     input = "body"
)

type output string

// outputs
const (
	bodyOutput  output = "body"
	errorOutput output = "error"
)

// Wrap the handler func into an HTTP handler
func (k *Koi) Wrap(handler interface{}) http.Handler {
	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		panic("koi: handler is not a function")
	}

	// get the function type
	typ := fn.Type()

	// inputs
	inArity := typ.NumIn()
	inputs := make([]input, inArity)
	var inputBody reflect.Type
	for i := range inputs {
		arg := typ.In(i)
		switch input(arg.String()) {
		case responseInput:
			inputs[i] = responseInput
			continue
		case contextInput:
			inputs[i] = contextInput
			continue
		case requestInput:
			inputs[i] = requestInput
			continue
		}
		if inputBody != nil {
			panic("koi: input must have exactly 1 body argument " + arg.String())
		}
		inputs[i] = bodyInput
		inputBody = arg
		continue
	}

	// outputs
	outArity := typ.NumOut()
	outputs := make([]output, outArity)
	var outputBody reflect.Type
	for o := range outputs {
		result := typ.Out(o)
		switch output(result.String()) {
		case errorOutput:
			outputs[o] = errorOutput
			continue
		}
		if outputBody != nil {
			panic("koi: input must have exactly 1 body result " + result.String())
		}
		outputs[o] = bodyOutput
		outputBody = result
		continue
	}

	return k.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepts := negotiate(r, k.Accepts, "application/json")

		// read the request body
		body, err := k.Read(r.Body)
		if err != nil {
			k.Write(w, accepts, BadRequest(err))
			return
		}

		// handle the inputs
		// TODO: can this be precomputed?
		var arguments = make([]reflect.Value, inArity)
		for i := range inputs {
			switch inputs[i] {
			case responseInput:
				arguments[i] = reflect.ValueOf(w)
			case requestInput:
				arguments[i] = reflect.ValueOf(r)
			case contextInput:
				arguments[i] = reflect.ValueOf(r.Context())
			case bodyInput:
				stct := reflect.New(inputBody)
				if err := json.Unmarshal(body, stct.Interface()); err != nil {
					k.Write(w, accepts, BadRequest(err))
					return
				}
				arguments[i] = stct.Elem()
			}
		}

		// handle the outputs
		results := fn.Call(arguments)
		var resultv, errorv reflect.Value
		for o, v := range results {
			switch outputs[o] {
			case bodyOutput:
				resultv = v
			case errorOutput:
				errorv = v
			}
		}

		// error
		if errorv.IsValid() && !errorv.IsNil() {
			k.Write(w, accepts, errorv.Interface())
			return
		}

		// ensure the result isn't empty
		var value interface{}
		if resultv.IsValid() {
			value = resultv.Interface()
		}

		// value
		k.Write(w, accepts, value)
	}))
}

// default write function
func defaultWrite(w http.ResponseWriter, accepts string, v interface{}) error {
	status := http.StatusOK

	// ignore nils
	if v == nil {
		w.WriteHeader(status)
		return nil
	}

	// handle errors
	switch e := v.(type) {
	case *Error:
		status = e.Status
	case error:
		// turn into a Koi error
		return defaultWrite(w, accepts, InternalServerError(e))
	}

	// format the body
	var body bytes.Buffer
	switch accepts {
	case "application/json":
		b, err := json.Marshal(v)
		if err != nil {
			return defaultWrite(w, accepts, BadRequest(err))
		}
		body.Write(b)
	case "text/html", "text/plain":
		body.WriteString(fmt.Sprintf("%v", v))
	default:
		return defaultWrite(w, accepts, BadRequest(fmt.Errorf("unhandled accept header: %s", accepts)))
	}

	// write out
	w.Header().Set("Content-Type", accepts)
	w.WriteHeader(status)
	w.Write(body.Bytes())

	return nil
	// fmt.Println("accepts", accepts)

	// // koi error
	// if e, ok := v.(Error); ok {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	w.WriteHeader(e.Status)
	// 	b, err := json.Marshal(e)
	// 	if err != nil {
	// 		http.Error(w, "Error marshalling output error", http.StatusInternalServerError)
	// 		return err
	// 	}
	// 	w.Write(b)
	// 	return nil
	// }

	// // all other errors
	// // TODO: unfold this function call
	// if err, ok := v.(error); ok {
	// 	http.Error(w, err.Error(), 500)
	// 	return nil
	// 	// return write(w, InternalServerError(err))
	// }

	// // all other response bodies
	// w.Header().Set("Content-Type", "application/json")
	// b, err := json.Marshal(v)
	// if err != nil {
	// 	return defaultWrite(w, accepts, BadRequest(err))
	// }
	// w.Write(b)

	// return nil
}

// compose multiple middleware into one
func compose(mm ...func(next http.Handler) http.Handler) func(next http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		if len(mm) == 0 {
			return h
		}
		for i := len(mm) - 1; i >= 0; i-- {
			h = mm[i](h)
		}
		return h
	}
}

// isError returns true if v implements the error interface.
func isError(v reflect.Value) bool {
	return v.Type().Implements(errorInterface)
}

// unroll unrolls and pointers of a type.
func unroll(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func unmarshalable(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		return t.Implements(unmarshalerInterface)
	}
	ptr := reflect.PtrTo(t)
	return ptr.Implements(unmarshalerInterface)
}

// Negotiate returns the best offered content type for the request's
// Accept header. If two offers match with equal weight, then the more specific
// offer is preferred.  For example, text/* trumps */*. If two offers match
// with equal weight and specificity, then the offer earlier in the list is
// preferred. If no offers match, then defaultOffer is returned.
//
// Based on: https://github.com/golang/gddo/blob/master/httputil/negotiate.go
func negotiate(r *http.Request, offers []string, defaultOffer string) string {
	bestOffer := defaultOffer
	bestQ := -1.0
	bestWild := 3
	specs := header.ParseAccept(r.Header, "Accept")
	for _, offer := range offers {
		for _, spec := range specs {
			switch {
			case spec.Q == 0.0:
				// ignore
			case spec.Q < bestQ:
				// better match found
			case spec.Value == "*/*":
				if spec.Q > bestQ || bestWild > 2 {
					bestQ = spec.Q
					bestWild = 2
					bestOffer = offer
				}
			case strings.HasSuffix(spec.Value, "/*"):
				if strings.HasPrefix(offer, spec.Value[:len(spec.Value)-1]) &&
					(spec.Q > bestQ || bestWild > 1) {
					bestQ = spec.Q
					bestWild = 1
					bestOffer = offer
				}
			default:
				if spec.Value == offer &&
					(spec.Q > bestQ || bestWild > 0) {
					bestQ = spec.Q
					bestWild = 0
					bestOffer = offer
				}
			}
		}
	}
	return bestOffer
}

// TODO: add later
// // MovedPermanently redirect
// func MovedPermanently(url string) *Redirect {
// 	return &Redirect{
// 		Status: http.StatusMovedPermanently,
// 		URL:    url,
// 	}
// }

// // TemporaryRedirect response
// func TemporaryRedirect(url string) *Redirect {
// 	return &Redirect{
// 		Status: http.StatusTemporaryRedirect,
// 		URL:    url,
// 	}
// }

// // Redirect struct
// type Redirect struct {
// 	Status int
// 	URL    string
// }

// BadRequest response.
func BadRequest(err error) *Error {
	return &Error{
		Status:  http.StatusBadRequest,
		Message: err.Error(),
	}
}

// Unauthorized response.
func Unauthorized(err error) error {
	return &Error{
		Status:  http.StatusUnauthorized,
		Message: err.Error(),
	}
}

// PaymentRequired response.
func PaymentRequired(err error) error {
	return &Error{
		Status:  http.StatusPaymentRequired,
		Message: err.Error(),
	}
}

// Forbidden response.
func Forbidden(err error) error {
	return &Error{
		Status:  http.StatusForbidden,
		Message: err.Error(),
	}
}

// NotFound response.
func NotFound(err error) error {
	return &Error{
		Status:  http.StatusNotFound,
		Message: err.Error(),
	}
}

// MethodNotAllowed response.
func MethodNotAllowed(err error) error {
	return &Error{
		Status:  http.StatusMethodNotAllowed,
		Message: err.Error(),
	}
}

// NotAcceptable response.
func NotAcceptable(err error) error {
	return &Error{
		Status:  http.StatusNotAcceptable,
		Message: err.Error(),
	}
}

// ProxyAuthRequired response.
func ProxyAuthRequired(err error) error {
	return &Error{
		Status:  http.StatusProxyAuthRequired,
		Message: err.Error(),
	}
}

// RequestTimeout response.
func RequestTimeout(err error) error {
	return &Error{
		Status:  http.StatusRequestTimeout,
		Message: err.Error(),
	}
}

// Conflict response.
func Conflict(err error) error {
	return &Error{
		Status:  http.StatusConflict,
		Message: err.Error(),
	}
}

// Gone response.
func Gone(err error) error {
	return &Error{
		Status:  http.StatusGone,
		Message: err.Error(),
	}
}

// LengthRequired response.
func LengthRequired(err error) error {
	return &Error{
		Status:  http.StatusLengthRequired,
		Message: err.Error(),
	}
}

// PreconditionFailed response.
func PreconditionFailed(err error) error {
	return &Error{
		Status:  http.StatusPreconditionFailed,
		Message: err.Error(),
	}
}

// RequestEntityTooLarge response.
func RequestEntityTooLarge(err error) error {
	return &Error{
		Status:  http.StatusRequestEntityTooLarge,
		Message: err.Error(),
	}
}

// RequestURITooLong response.
func RequestURITooLong(err error) error {
	return &Error{
		Status:  http.StatusRequestURITooLong,
		Message: err.Error(),
	}
}

// UnsupportedMediaType response.
func UnsupportedMediaType(err error) error {
	return &Error{
		Status:  http.StatusBadRequest,
		Message: err.Error(),
	}
}

// RequestedRangeNotSatisfiable response.
func RequestedRangeNotSatisfiable(err error) error {
	return &Error{
		Status:  http.StatusRequestedRangeNotSatisfiable,
		Message: err.Error(),
	}
}

// ExpectationFailed response.
func ExpectationFailed(err error) error {
	return &Error{
		Status:  http.StatusExpectationFailed,
		Message: err.Error(),
	}
}

// Teapot response.
func Teapot(err error) error {
	return &Error{
		Status:  http.StatusTeapot,
		Message: err.Error(),
	}
}

// InternalServerError response.
func InternalServerError(err error) error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Message: err.Error(),
	}
}

// NotImplemented response.
func NotImplemented(err error) error {
	return &Error{
		Status:  http.StatusNotImplemented,
		Message: err.Error(),
	}
}

// BadGateway response.
func BadGateway(err error) error {
	return &Error{
		Status:  http.StatusBadGateway,
		Message: err.Error(),
	}
}

// ServiceUnavailable response.
func ServiceUnavailable(err error) error {
	return &Error{
		Status:  http.StatusServiceUnavailable,
		Message: err.Error(),
	}
}

// GatewayTimeout response.
func GatewayTimeout(err error) error {
	return &Error{
		Status:  http.StatusGatewayTimeout,
		Message: err.Error(),
	}
}

// HTTPVersionNotSupported response.
func HTTPVersionNotSupported(err error) error {
	return &Error{
		Status:  http.StatusHTTPVersionNotSupported,
		Message: err.Error(),
	}
}

// Error is an koi error.
type Error struct {
	Status  int    `json:"-"`
	Message string `json:"message,omitempty"`
}

// Error implementation.
func (e Error) Error() string {
	return e.Message
}
