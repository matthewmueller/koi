package koi_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/matthewmueller/diff"
	"github.com/matthewmueller/koi"
	"github.com/matthewmueller/koi/invalid"
)

func equal(t testing.TB, h http.Handler, r *http.Request, expect string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	w := rec.Result()
	dump, err := httputil.DumpResponse(w, true)
	if err != nil {
		if err.Error() != expect {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	diff.TestHTTP(t, string(dump), expect)
}

func TestJsonGetEmpty(t *testing.T) {
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in struct{}) (out struct{}, err error) {
		return out, nil
	})
	req := httptest.NewRequest("GET", "/", nil)
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{}
	`)
}

func TestJsonPostEmpty(t *testing.T) {
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in struct{}) (out struct{}, err error) {
		return out, nil
	})
	req := httptest.NewRequest("POST", "/", nil)
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{}
	`)
}

func TestJsonGetInOutZero(t *testing.T) {
	type InOut struct {
		Name string `json:"name,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in InOut) (out InOut, err error) {
		return in, nil
	})
	req := httptest.NewRequest("GET", "/", nil)
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{}
	`)
}

func TestJsonGetInOut(t *testing.T) {
	type InOut struct {
		Name string `json:"name,omitzero"`
		Age  int    `json:"age,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in InOut) (out InOut, err error) {
		return in, nil
	})
	req := httptest.NewRequest("GET", "/?name=matt&age=10", nil)
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{"name":"matt","age":10}
	`)
}

func TestJsonGetPtrInOut(t *testing.T) {
	type In struct {
		Name string `json:"name,omitzero"`
		Age  int    `json:"age,omitzero"`
	}
	type Out struct {
		Name string `json:"name,omitzero"`
		Age  int    `json:"age,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in *In) (out *Out, err error) {
		return &Out{in.Name, in.Age * 2}, nil
	})
	req := httptest.NewRequest("GET", "/?name=matt&age=10", nil)
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{"name":"matt","age":20}
	`)
}

func TestJsonPostPtrInOut(t *testing.T) {
	type InOut struct {
		Name string `json:"name,omitzero"`
		Age  int    `json:"age,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in *InOut) (out *InOut, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/?name=matt", bytes.NewBufferString(`{"age":10}`))
	req.Header.Set("Content-Type", "application/json")
	equal(t, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{"name":"matt","age":10}
	`)
}

func TestJsonPostUnsupportedMediaType(t *testing.T) {
	type In struct {
		Name string `json:"name,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in In) (out In, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString("name=matt"))
	req.Header.Set("Content-Type", "text/plain")
	equal(t, handler, req, `
		HTTP/1.1 400 Bad Request
		Connection: close
		Content-Type: application/json

		{"error":"unsupported media type \"text/plain\""}
	`)
}

func TestJsonPostMissingContentType(t *testing.T) {
	type In struct {
		Name string `json:"name,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in In) (out In, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"matt"}`))
	equal(t, handler, req, `
		HTTP/1.1 400 Bad Request
		Connection: close
		Content-Type: application/json

		{"error":"unsupported media type"}
	`)
}

func TestJsonPostDuplicateJSONField(t *testing.T) {
	type In struct {
		Name string `json:"name,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in In) (out In, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/?name=query", bytes.NewBufferString(`{"name":"body"}`))
	req.Header.Set("Content-Type", "application/json")
	equal(t, handler, req, `
		HTTP/1.1 400 Bad Request
		Connection: close
		Content-Type: application/json

		{"error":"duplicate request field \"name\""}
	`)
}

func TestJsonPostDuplicateFormField(t *testing.T) {
	type In struct {
		Name string `json:"name,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in In) (out In, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/?name=query", bytes.NewBufferString("name=body"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	equal(t, handler, req, `
		HTTP/1.1 400 Bad Request
		Connection: close
		Content-Type: application/json

		{"error":"duplicate request field \"name\""}
	`)
}

func TestJsonPostError(t *testing.T) {
	type InOut struct {
		Name string `json:"name,omitzero"`
		Age  int    `json:"age,omitzero"`
	}
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in *InOut) (out *InOut, err error) {
		return nil, fmt.Errorf("oh no")
	})
	req := httptest.NewRequest("POST", "/?name=matt", bytes.NewBufferString(`{"age":10}`))
	req.Header.Set("Content-Type", "application/json")
	equal(t, handler, req, `
		HTTP/1.1 500 Internal Server Error
		Connection: close
		Content-Type: application/json

		{"error":"oh no"}
	`)
}

type InOut struct {
	Email string `json:"email,omitzero" validate:"email,required"`
}

func (in *InOut) Validate() (err error) {
	if in.Email == "" {
		err = invalid.Join(err, "email", "is required")
	}
	return err
}

func TestJsonPostValidationError(t *testing.T) {
	handler := koi.JSON(func(w http.ResponseWriter, r *http.Request, in *InOut) (out *InOut, err error) {
		return in, nil
	})
	req := httptest.NewRequest("POST", "/?name=matt", bytes.NewBufferString(`{"age":10}`))
	req.Header.Set("Content-Type", "application/json")
	equal(t, handler, req, `
		HTTP/1.1 400 Bad Request
		Connection: close
		Content-Type: application/json

		{"error":"invalid email","fields":{"email":["is required"]}}
	`)
}
