package koi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/bmizerany/pat"
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
		test     func(t *testing.T) http.Handler
		expected string
	}{
		{
			name:    "fn(value)",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) {
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
			test: func(t *testing.T) http.Handler {
				a := &A{t: t, Name: "Matt"}
				k := koi.New()
				return k.Wrap(a.Hello)
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			null
		`,
		},
		{
			name:    "fn(val) err",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) error {
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
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) error {
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
			name:    "fn(in) (out, err)",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) (r string, e error) {
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
			name:    "fn(in) (out, err) with error",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) (r string, e error) {
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
			name:    "fn(*in) (result, err)",
			request: req("POST", "/", `{"hello":null}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v *struct{ Hello string }) (r string, e error) {
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
			name:    "fn(*in) (*result, error)",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v *struct{ Hello string }) (r *string, e error) {
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
			name:    "fn(res, req)",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request) {
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
			name:    "fn(res, req) err",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request) error {
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
			name:    "fn(res, req) err: with error",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request) error {
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
			name:    "fn(res, req, in)",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) {
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
			name:    "fn(res, req, in) err",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
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
			name:    "fn(res, req, in) err: error > write",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
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
			name:    "fn(res, req, in) (out, err): out > write",
			request: req("POST", "/", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) (s struct{ Ok bool }, err error) {
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
			name:    "malformed",
			request: req("POST", "/matt", `{hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(v struct{ Hello string }) {})
			},
			expected: `
			HTTP/1.1 400 Bad Request
			Connection: close
			Content-Type: application/json

			{"type":"malformed_request","message":"invalid body, expected object"}
		`,
		},
		{
			name:    "json.RawMessage",
			request: req("POST", "/matt", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New()
				return k.Wrap(func(in json.RawMessage) json.RawMessage {
					return in
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"hello":"hi"}
		`,
		},
		{
			name:    "path params to query params",
			request: req("POST", "/matt/mueller", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				router := pat.New()
				k := koi.New()
				router.Post("/:first/:last", k.Wrap(func(r *http.Request) string {
					q := r.URL.Query()
					return q.Get(":first") + " " + q.Get(":last")
				}))
				return router
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"matt mueller"
		`,
		},
		{
			name:    "with empty middleware",
			request: req("POST", "/matt/mueller", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New(
					func(next http.Handler) http.Handler {
						return next
					},
					func(next http.Handler) http.Handler {
						return next
					},
				)
				return k.Wrap(func(r *http.Request) string {
					return "ok"
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"ok"
		`,
		},
		{
			name:    "with middleware",
			request: req("POST", "/matt/mueller", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Add("Content-Type", "application/json")
						w.Write([]byte(`"lol"`))
					})
				})
				return k.Wrap(func(r *http.Request) string {
					return "ok"
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			"lol"
		`,
		},
		{
			name:    "with multiple writing middleware",
			request: req("POST", "/matt/mueller", `{"hello":"hi"}`),
			test: func(t *testing.T) http.Handler {
				k := koi.New(
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Add("Content-Type", "application/json")
							w.Write([]byte(`{`))
							next.ServeHTTP(w, r)
						})
					},
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Write([]byte(`}`))
						})
					},
				)
				return k.Wrap(func(r *http.Request) string {
					return "ok"
				})
			},
			expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{}
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
			dif(t, test.expected, string(buf))
		})
	}
}

// diff two results
func dif(t testing.TB, expected, actual string) {
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
