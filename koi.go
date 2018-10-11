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

	"github.com/julienschmidt/httprouter"

	validator "gopkg.in/go-playground/validator.v9"
)

type route struct {
	fn         reflect.Value
	numIn      int
	numOut     int
	param      reflect.Type
	result     reflect.Type
	error      bool
	request    bool
	koiRequest bool
	response   bool
	context    bool
}

// Request that includes parameters
type Request struct {
	*http.Request
	params httprouter.Params
}

// Param returns a url parameter
func (r *Request) Param(name string) string {
	return r.params.ByName(name)
}

// options for koi
type options struct {
	Read     func(r io.Reader) ([]byte, error)
	Validate func(s interface{}) error
	Write    func(w http.ResponseWriter, v interface{}) error
}

// Option for koi
type Option func(*options)

// Read option allows you to customize how the body
// is read. Defaults to ioutil.ReadAll(r.Body)
func Read(fn func(r io.Reader) ([]byte, error)) Option {
	return func(opts *options) {
		opts.Read = fn
	}
}

// Validator sets a custom input validator. Defaults to using
// "gopkg.in/go-playground/validator.v9".Struct
func Validator(fn func(s interface{}) error) Option {
	return func(opts *options) {
		opts.Validate = fn
	}
}

// Write option customizes how to write the data back
func Write(fn func(w http.ResponseWriter, v interface{}) error) Option {
	return func(opts *options) {
		opts.Write = fn
	}
}

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

// Koi struct
type Koi struct {
	http.Handler
	router *httprouter.Router
	o      *options
}

// New koi middleware
func New(opts ...func(*options)) *Koi {
	r := httprouter.New()
	o := &options{
		Read:     ioutil.ReadAll,
		Validate: validator.New().Struct,
		Write:    write,
	}
	for _, opt := range opts {
		opt(o)
	}
	return &Koi{r, r, o}
}

// Get handles GET requests to path
func (k *Koi) Get(path string, handler ...interface{}) *Koi {
	k.router.Handle("GET", path, k.wrap(handler))
	return k
}

// Post handles POST requests to path
func (k *Koi) Post(path string, handler ...interface{}) *Koi {
	k.router.Handle("POST", path, k.wrap(handler))
	return k
}

// Put handles PUT requests to path
func (k *Koi) Put(path string, handler ...interface{}) *Koi {
	k.router.Handle("PUT", path, k.wrap(handler))
	return k
}

// Patch handles PATCH requests to path
func (k *Koi) Patch(path string, handler ...interface{}) *Koi {
	k.router.Handle("PATCH", path, k.wrap(handler))
	return k
}

// Delete handles DELETE requests to path
func (k *Koi) Delete(path string, handler ...interface{}) *Koi {
	k.router.Handle("DELETE", path, k.wrap(handler))
	return k
}

// Head handles HEAD requests to path
func (k *Koi) Head(path string, handler ...interface{}) *Koi {
	k.router.Handle("HEAD", path, k.wrap(handler))
	return k
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
		case "*koi.Request":
			r.koiRequest = true
			continue
		case "context.Context":
			r.context = true
			continue
		}
		if unrollPointer(arg).Kind() == reflect.Struct {
			r.param = arg
			continue
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
		if unrollPointer(result).Kind() == reflect.Struct {
			r.result = result
			continue
		}
	}

	return r
}

type handle func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc)

// Compose the handlers
func compose(fns ...handle) (next http.HandlerFunc) {
	next = func(w http.ResponseWriter, r *http.Request) {}
	for i := len(fns) - 1; i >= 0; i-- {
		i := i
		prev := next
		called := false
		next = func(w http.ResponseWriter, r *http.Request) {
			if called {
				return
			}
			called = true
			fns[i](w, r, prev)
			prev(w, r)
		}
	}
	return next
}

func (k *Koi) middleware(handler interface{}) handle {
	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		panic("middleware must be a function")
	}
	t := fn.Type()
	n := t.NumIn()
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		params := make([]reflect.Value, n)
		switch n {
		case 0:
			fn.Call(params)
		case 1:
			params[0] = reflect.ValueOf(w)
			fn.Call(params)
		case 2:
			params[0] = reflect.ValueOf(w)
			params[1] = reflect.ValueOf(r)
			fn.Call(params)
		default:
			params[0] = reflect.ValueOf(w)
			params[1] = reflect.ValueOf(r)
			params[2] = reflect.ValueOf(next)
			fn.Call(params)
		}
	}
}

func (k *Koi) wrap(handlers []interface{}) httprouter.Handle {
	var (
		middlewares []interface{}
		handler     interface{}
	)
	switch len(handlers) {
	case 0:
		panic("method must have atleast one handler")
	case 1:
		handler = handlers[0]
	default:
		l := len(handlers) - 1
		middlewares = handlers[0:l]
		handler = handlers[l]
	}

	var fns []handle
	if len(middlewares) > 0 {
		for _, middleware := range middlewares {
			fns = append(fns, k.middleware(middleware))
		}
	}

	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		panic("not a function")
	}
	route := k.route(fn)

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		fn := func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) { k.respond(route, w, r, p) }
		fns = append(fns, fn)
		compose(fns...)(w, r)
	}
}

// Wrap the handler in middleware
func (k *Koi) Wrap(handler interface{}) http.HandlerFunc {
	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		panic("not a function")
	}

	route := k.route(fn)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		k.respond(route, w, r, nil)
	})
}

func (k *Koi) respond(route *route, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// read the request body
	body, err := k.o.Read(r.Body)
	if err != nil {
		k.o.Write(w, NewError(400, "malformed_request", "Request body must be present"))
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
	if route.koiRequest {
		params = append(params, reflect.ValueOf(&Request{r, p}))
	}
	if route.param != nil {
		stct := reflect.New(route.param)
		if err := json.Unmarshal(body, stct.Interface()); err != nil {
			k.o.Write(w, NewError(400, "malformed_request", fmt.Sprintf("invalid body, expected %s", jsonTypeName(route.param))))
			return
		}
		if err := validate(route.param, stct, k.o.Validate); err != nil {
			k.o.Write(w, NewError(400, "invalid_input", err.Error()))
			return
		}
		params = append(params, stct.Elem())
	}

	// TODO: we should be able to catch this on startup
	if route.numIn != len(params) {
		k.o.Write(w, NewError(500, "bad_handler_signature", "handler function parameter mismatch"))
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
		k.o.Write(w, e)
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

	k.o.Write(w, out)
}

type bufferedWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *bufferedWriter) Write(b []byte) (n int, err error) {
	w.buf.Write(b)
	return len(b), nil
}

func validate(t reflect.Type, v reflect.Value, check func(s interface{}) error) error {
	switch t.Kind() {
	case reflect.Struct:
		return check(v.Interface())
	case reflect.Ptr:
		return validate(unrollPointer(t), v.Elem(), check)
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Elem().Len(); i++ {
			item := v.Elem().Index(i)
			if err := validate(item.Type(), item, check); err != nil {
				return err
			}
		}
	}
	return nil
}

var (
	// errorInterface is the error interface.
	errorInterface = reflect.TypeOf((*error)(nil)).Elem()

	// contextInterface is the context interface.
	contextInterface = reflect.TypeOf((*context.Context)(nil)).Elem()
)

// isError returns true if v implements the error interface.
func isError(v reflect.Value) bool {
	return v.Type().Implements(errorInterface)
}

// jsonTypeName returns the JSON name of the corresponding Go type.
func jsonTypeName(t reflect.Type) string {
	switch unrollPointer(t).Kind() {
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

// unrollPointer unrolls and pointers of a type.
func unrollPointer(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
