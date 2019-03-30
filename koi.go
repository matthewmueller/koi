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
)

var (
	// errorInterface is the error interface.
	errorInterface = reflect.TypeOf((*error)(nil)).Elem()

	// contextInterface is the context interface.
	contextInterface = reflect.TypeOf((*context.Context)(nil)).Elem()

	// unmarshaler interface is the json.Unmarshaler interface.
	unmarshalerInterface = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
)

type route struct {
	fn       reflect.Value
	numIn    int
	numOut   int
	param    reflect.Type
	result   reflect.Type
	error    bool
	request  bool
	response bool
	context  bool
}

// default write function
func write(w http.ResponseWriter, v interface{}) error {
	// koi error
	if e, ok := v.(Error); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(e.Status)
		b, err := json.Marshal(e)
		if err != nil {
			http.Error(w, "Error marshalling output error", http.StatusInternalServerError)
			return err
		}
		w.Write(b)
		return nil
	}

	// all other errors
	if e, ok := v.(error); ok {
		return write(w, NewError(500, "", e.Error()))
	}

	// all other response bodies
	b, err := json.Marshal(v)
	if err != nil {
		return write(w, NewError(400, "malformed_response", err.Error()))
	}
	w.Write(b)

	return nil
}

// New koi with optional middleware
func New(middleware ...Middleware) *Koi {
	return &Koi{
		Middleware: compose(middleware...),
		Read:       ioutil.ReadAll,
		Write:      write,
	}
}

// Middleware for Koi
type Middleware func(next http.Handler) http.Handler

// Koi struct
type Koi struct {
	Middleware Middleware
	Read       func(r io.Reader) ([]byte, error)
	Write      func(w http.ResponseWriter, v interface{}) error
}

func (k *Koi) route(fn reflect.Value) *route {
	r := &route{}
	t := fn.Type()
	r.fn = fn

	r.numIn = t.NumIn()
	params := make([]reflect.Value, r.numIn)
	for i := range params {
		arg := t.In(i)
		switch arg.String() {
		case "http.ResponseWriter":
			r.response = true
			continue
		case "*http.Request":
			r.request = true
			continue
		case "context.Context":
			r.context = true
			continue
		}

		if unroll(arg).Kind() == reflect.Struct || unmarshalable(arg) {
			r.param = arg
		}
	}

	results := make([]reflect.Value, t.NumOut())
	for i := range results {
		result := t.Out(i)
		switch result.String() {
		case "error":
			r.error = true
			continue
		}
		if unroll(result).Kind() == reflect.Struct {
			r.result = result
			continue
		}
	}

	return r
}

// compose multiple middleware into one
func compose(mm ...Middleware) Middleware {
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

// Wrap the handler in middleware
func (k *Koi) Wrap(handler interface{}) http.Handler {
	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		panic("not a function")
	}
	route := k.route(fn)
	handle := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.respond(route, w, r)
	})
	return k.Middleware(handle)
}

func (k *Koi) respond(route *route, w http.ResponseWriter, r *http.Request) {
	// read the request body
	body, err := k.Read(r.Body)
	if err != nil {
		k.Write(w, NewError(400, "malformed_request", "Request body must be present"))
		return
	}
	r.Body.Close()

	bw := bufferedWriter{ResponseWriter: w}

	var params []reflect.Value
	if route.response {
		params = append(params, reflect.ValueOf(&bw))
	}
	if route.request {
		params = append(params, reflect.ValueOf(r))
	}
	if route.param != nil {
		stct := reflect.New(route.param)
		if err := json.Unmarshal(body, stct.Interface()); err != nil {
			k.Write(w, NewError(400, "malformed_request", fmt.Sprintf("invalid body, expected %s", jsonTypeName(route.param))))
			return
		}
		params = append(params, stct.Elem())
	}

	// TODO: we should be able to catch this on startup
	if route.numIn != len(params) {
		k.Write(w, NewError(500, "bad_handler_signature", "handler function parameter mismatch"))
		return
	}

	results := route.fn.Call(params)
	var resValue, errValue reflect.Value
	for _, v := range results {
		if isError(v) {
			errValue = v
		} else {
			resValue = v
		}
	}

	// error
	if errValue.IsValid() && !errValue.IsNil() {
		w.Header().Set("Content-Type", "application/json")
		e := errValue.Interface().(error)
		k.Write(w, e)
		return
	}

	if route.result == nil {
		buffer := bw.buf.Bytes()
		if len(buffer) > 0 {
			w.Write(buffer)
			return
		}
	}

	// value
	w.Header().Set("Content-Type", "application/json")
	var out interface{}

	if resValue.IsValid() {
		out = resValue.Interface()
	}

	k.Write(w, out)
}

type bufferedWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *bufferedWriter) Write(b []byte) (n int, err error) {
	w.buf.Write(b)
	return len(b), nil
}

// isError returns true if v implements the error interface.
func isError(v reflect.Value) bool {
	return v.Type().Implements(errorInterface)
}

// jsonTypeName returns the JSON name of the corresponding Go type.
func jsonTypeName(t reflect.Type) string {
	switch unroll(t).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array of " + jsonTypeName(t.Elem()) + "s"
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Map:
		return "object"
	case reflect.Struct:
		return "object"
	default:
		return "unknown"
	}
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

// Error is an koi error.
type Error struct {
	Status  int    `json:"-"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// NewError returns a new RPC error with HTTP status code, kind and message.
func NewError(status int, kind, message string) error {
	return Error{
		Status:  status,
		Type:    kind,
		Message: message,
	}
}

// Error implementation.
func (e Error) Error() string {
	return e.Message
}
