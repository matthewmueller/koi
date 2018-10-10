package koi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"

	validator "gopkg.in/go-playground/validator.v9"
)

type handler struct {
	param    reflect.Type
	result   reflect.Type
	error    bool
	request  bool
	response bool
	context  bool
}

// OutputResult is a value result of a function call.
type OutputResult struct {
	Result interface{} `json:"result"`
}

var readBody = ioutil.ReadAll
var valid = validator.New()

// New koi wrapper
func New(v interface{}) http.HandlerFunc {
	fn := reflect.ValueOf(v)
	if fn.Kind() != reflect.Func {
		panic("not a function")
	}

	t := fn.Type()
	h := handler{}

	params := make([]reflect.Value, t.NumIn())
	for i := range params {
		arg := t.In(i)
		switch arg.String() {
		case "http.ResponseWriter":
			h.response = true
			continue
		case "*http.Request":
			h.request = true
			continue
		case "context.Context":
			h.context = true
			continue
		}
		if arg.Kind() == reflect.Struct {
			h.param = arg
			continue
		}
	}

	results := make([]reflect.Value, t.NumOut())
	for i := range results {
		result := t.Out(i)
		switch result.String() {
		case "error":
			h.error = true
			continue
		}
		if result.Kind() == reflect.Struct {
			h.result = result
			continue
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// read the request body
		body, err := readBody(r.Body)
		if err != nil {
			sendError(w, NewError(400, "malformed_request", "Request body must be present"))
			return
		}
		r.Body.Close()

		bw := bufferedWriter{ResponseWriter: w}

		var params []reflect.Value
		if h.response {
			params = append(params, reflect.ValueOf(&bw))
		}
		if h.request {
			params = append(params, reflect.ValueOf(r))
		}
		if h.param != nil {
			stct := reflect.New(h.param)
			if err := json.Unmarshal(body, stct.Interface()); err != nil {
				sendError(w, NewError(400, "malformed_request", fmt.Sprintf("invalid body, expected %s", jsonTypeName(h.param))))
				return
			}
			if err := validate(h.param, stct); err != nil {
				sendError(w, NewError(400, "invalid_input", err.Error()))
				return
			}
			params = append(params, stct.Elem())
		}

		results := fn.Call(params)
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
			sendError(w, e)
			return
		}

		if h.result == nil {
			buffer := bw.buf.Bytes()
			if len(buffer) > 0 {
				w.Write(buffer)
				return
			}
		}

		// value
		w.Header().Set("Content-Type", "application/json")
		var out OutputResult

		if resValue.IsValid() {
			out.Result = resValue.Interface()
		}

		b, err := json.Marshal(out)
		if err != nil {
			sendError(w, NewError(400, "malformed_response", err.Error()))
			return
		}

		w.Write(b)
	})
}

type bufferedWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *bufferedWriter) Write(b []byte) (n int, err error) {
	w.buf.Write(b)
	return len(b), nil
}

func sendError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	e, ok := err.(Error)
	if !ok {
		e = NewError(500, "", err.Error()).(Error)
	}

	w.WriteHeader(e.Status)
	b, err := json.Marshal(OutputError{e})
	if err != nil {
		http.Error(w, "Error marshalling output error", http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func validate(t reflect.Type, v reflect.Value) error {
	switch t.Kind() {
	case reflect.Ptr:
		return validate(unrollPointer(t), v.Elem())
	case reflect.Struct:
		return valid.Struct(v.Interface())
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Elem().Len(); i++ {
			item := v.Elem().Index(i)
			if err := validate(item.Type(), item); err != nil {
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
