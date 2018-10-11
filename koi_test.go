package koi_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/matthewmueller/diff"
	"github.com/matthewmueller/koi"
	"github.com/renstrom/dedent"
	"github.com/tj/assert"
)

func req(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, bytes.NewBufferString(body))
}

type A struct {
	t    testing.TB
	Name string
}

func (a *A) Hello(v struct{ Hello string }) {
	assert.Equal(a.t, a.Name, "Matt")
	assert.Equal(a.t, v.Hello, "hi")
}

func TestWrap(t *testing.T) {
	var tests = []struct {
		name     string
		request  *http.Request
		test     func(t *testing.T) http.HandlerFunc
		expected string
	}{
		{
			name:    "v->",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) {
					assert.Equal(t, v.Hello, "hi")
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			null
		`,
		},
		{
			name:    "method test",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				a := &A{t: t, Name: "Matt"}
				return koi.New().Wrap(a.Hello)
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			null
		`,
		},
		{
			name:    "v->e",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) error {
					assert.Equal(t, v.Hello, "hi")
					return nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			null
		`,
		},
		{
			name:    "returns error",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) error {
					assert.Equal(t, v.Hello, "hi")
					return errors.New("oh noz")
				})
			},
			expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"message":"oh noz"}
		`,
		},
		{
			name:    "v->r,e",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) (r string, e error) {
					return v.Hello, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"hi"
		`,
		},
		{
			name:    "v->r,e with error",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) (r string, e error) {
					return v.Hello, errors.New("Oh noz.")
				})
			},
			expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"message":"Oh noz."}
		`,
		},
		{
			name:    "v->r,e with input ptr",
			request: req("POST", "/", `{"hello":null}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v *struct{ Hello string }) (r string, e error) {
					return v.Hello, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			""
		`,
		},
		{
			name:    "v->r,e with output ptr",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v *struct{ Hello string }) (r *string, e error) {
					return &v.Hello, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"hi"
		`,
		},
		{
			name:    "w,r->",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method))
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: text/plain; charset=utf-8
			X-Test: machina

			POST
		`,
		},
		{
			name:    "w,r->e",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request) error {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method))
					return nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: text/plain; charset=utf-8
			X-Test: machina

			POST
		`,
		},
		{
			name:    "w,r->e with error",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request) error {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method))
					return errors.New("oh noz")
				})
			},
			expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json
			X-Test: machina

			{"message":"oh noz"}
		`,
		},
		{
			name:    "w,r,v->",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method + " " + v.Hello))
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: text/plain; charset=utf-8
			X-Test: machina

			POST hi
		`,
		},
		{
			name:    "w,r,v->e",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
					assert.Equal(t, "hi", v.Hello)
					return nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			null
		`,
		},
		{
			name:    "w,r,v->e error beats write",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
					assert.Equal(t, "hi", v.Hello)
					w.Write([]byte("whaddup"))
					return errors.New("oh noz")
				})
			},
			expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"message":"oh noz"}
		`,
		},
		{
			name:    "w,r,v->r,e result beats write",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) (s struct{ Ok bool }, err error) {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method))
					s.Ok = true
					return s, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json
			X-Test: machina

			{"Ok":true}
		`,
		},
		{
			name:    "w,r,v->r,e result beats write",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) (s struct{ Ok bool }, err error) {
					w.Header().Add("x-test", "machina")
					w.Write([]byte(r.Method))
					s.Ok = true
					return s, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json
			X-Test: machina

			{"Ok":true}
		`,
		},
		{
			name:    "kr,v->r,e",
			request: req("POST", "/matt", `{"hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(r *koi.Request, v struct{ Hello string }) (string, error) {
					return r.Method, nil
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"POST"
		`,
		},
		{
			name:    "malformed",
			request: req("POST", "/matt", `{hello":"hi"}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct{ Hello string }) {})
			},
			expected: `
			HTTP/1.1 400 Bad Request
			Connection: close
			Content-Type: application/json

			{"type":"malformed_request","message":"invalid body, expected object"}
		`,
		},
		{
			name:    "invalid input",
			request: req("POST", "/matt", `{"hello":""}`),
			test: func(t *testing.T) http.HandlerFunc {
				return koi.New().Wrap(func(v struct {
					Hello string `validate:"required"`
				}) {
				})
			},
			expected: `
			HTTP/1.1 400 Bad Request
			Connection: close
			Content-Type: application/json

			{"type":"invalid_input","message":"Key: 'Hello' Error:Field validation for 'Hello' failed on the 'required' tag"}
		`,
		},
		// {
		// 	name: "(w,r) (e)",
		// 	fn: func(w http.ResponseWriter, r *http.Request) {
		// 	},
		// },
		// {
		// 	name: "(w,r) (r,e)",
		// 	fn: func(w http.ResponseWriter, r *http.Request) {
		// 	},
		// },
		// {
		// 	name: "(ctx,v) ()",
		// 	fn:   func(ctx context.Context, v struct{}) {},
		// },
		// {
		// 	name: "(ctx,v) (e)",
		// },
		// {
		// 	name: "(ctx,v) (r,e)",
		// 	fn: func(ctx context.Context, v struct{}),
		// },
		// {
		// 	name: "(r,v) ()",
		// },
		// {
		// 	name: "(r,v) (e)",
		// },
		// {
		// 	name: "(r,v) (r,e)",
		// },
		// {
		// 	name: "(kr,v) ()",
		// },
		// {
		// 	name: "(kr,v) (e)",
		// },
		// {
		// 	name: "(kr,v) (r,e)",
		// },
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := test.test(t)
			w := httptest.NewRecorder()
			fn(w, test.request)
			res := w.Result()
			buf, err := httputil.DumpResponse(res, true)
			assert.NoError(t, err)
			difference(t, test.expected, string(buf))
		})
	}
}

func TestHandler(t *testing.T) {
	var tests = []struct {
		name     string
		request  *http.Request
		test     func(t testing.TB) http.Handler
		expected string
	}{
		{
			name:    "post /:first/:last",
			request: req("POST", "/matt/mueller", `{"hello":"hola"}`),
			test: func(t testing.TB) http.Handler {
				type Input struct {
					Hello string `json:"hello,omitempty" validate:"required"`
				}
				type Output struct {
					Goodbye string `json:"goodbye,omitempty"`
				}
				return koi.New().Post("/:first/:last", func(r *koi.Request, v Input) (o Output, e error) {
					assert.Equal(t, v.Hello, "hola")
					o.Goodbye = fmt.Sprintf("adios %s %s!", r.Param("first"), r.Param("last"))
					return o, nil
				})
			},
			expected: `
				HTTP/1.1 200 OK
				Connection: close
				Content-Type: application/json

				{"goodbye":"adios matt mueller!"}
			`,
		},
		{
			name:    "post /*name",
			request: req("POST", "/matt/mueller", `{"hello":"hola"}`),
			test: func(t testing.TB) http.Handler {
				type Input struct {
					Hello string `json:"hello,omitempty" validate:"required"`
				}
				type Output struct {
					Goodbye string `json:"goodbye,omitempty"`
				}
				return koi.New().Post("/*name", func(r *koi.Request, v Input) (o Output, e error) {
					assert.Equal(t, v.Hello, "hola")
					o.Goodbye = fmt.Sprintf("adios %s!", r.Param("name"))
					return o, nil
				})
			},
			expected: `
				HTTP/1.1 200 OK
				Connection: close
				Content-Type: application/json

				{"goodbye":"adios /matt/mueller!"}
			`,
		},
		{
			name:    "post 404",
			request: req("POST", "/matt/mueller", `{"hello":"hola"}`),
			test: func(t testing.TB) http.Handler {
				type Input struct {
					Hello string `json:"hello,omitempty" validate:"required"`
				}
				type Output struct {
					Goodbye string `json:"goodbye,omitempty"`
				}
				return koi.New().Post("/", func(r *koi.Request, v Input) (o Output, e error) {
					assert.Equal(t, v.Hello, "hola")
					o.Goodbye = fmt.Sprintf("adios %s!", r.Param("name"))
					return o, nil
				})
			},
			expected: `
				HTTP/1.1 404 Not Found
				Connection: close
				Content-Type: text/plain; charset=utf-8
				X-Content-Type-Options: nosniff

				404 page not found
			`,
		},
		{
			name:    "post / with middleware",
			request: req("POST", "/", `{"hello":"hola"}`),
			test: func(t testing.TB) http.Handler {
				var steps []string

				another := func() {
					steps = append(steps, "d ->")
				}

				return koi.New().Post("/",
					func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						steps = append(steps, "a ->")
						next(w, r)
						steps = append(steps, "a <-")
						assert.Equal(t, "a ->, b ->, c ->, d ->, a <-", strings.Join(steps, ", "))
					},
					func(w http.ResponseWriter, r *http.Request) {
						steps = append(steps, "b ->")
					},
					func(w http.ResponseWriter) {
						w.Header().Add("X-cool", "story bro")
						steps = append(steps, "c ->")
					},
					another,
					func() (s string) {
						assert.Equal(t, "a ->, b ->, c ->, d ->", strings.Join(steps, ", "))
						return "hi"
					})
			},
			expected: `
				HTTP/1.1 200 OK
				Connection: close
				Content-Type: application/json
				X-Cool: story bro

				"hi"
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := test.test(t)
			w := httptest.NewRecorder()
			fn.ServeHTTP(w, test.request)
			res := w.Result()
			buf, err := httputil.DumpResponse(res, true)
			assert.NoError(t, err)
			difference(t, test.expected, string(buf))
		})
	}
}

// diff two results
func difference(t testing.TB, expected, actual string) {
	expected = strings.Replace(strings.TrimSpace(dedent.Dedent(expected)), "\n", "\r\n", -1)
	actual = strings.TrimSpace(dedent.Dedent(actual))

	if expected == actual {
		return
	}

	var b bytes.Buffer
	b.WriteString("\n\x1b[4mExpected\x1b[0m:\n")
	b.WriteString(expected)
	b.WriteString("\n\n")
	b.WriteString("\x1b[4mActual\x1b[0m: \n")
	b.WriteString(actual)
	b.WriteString("\n\n")
	b.WriteString("\x1b[4mDifference\x1b[0m: \n")
	b.WriteString(diff.String(expected, actual))
	b.WriteString("\n")
	t.Fatal(b.String())
}
