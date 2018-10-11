package koi_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
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
			return koi.New()(func(v struct{ Hello string }) {
				assert.Equal(t, v.Hello, "hi")
			})
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":null}
		`,
	},
	{
		name:    "method test",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			a := &A{t: t, Name: "Matt"}
			return koi.New()(a.Hello)
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":null}
		`,
	},
	{
		name:    "v->e",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(v struct{ Hello string }) error {
				assert.Equal(t, v.Hello, "hi")
				return nil
			})
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":null}
		`,
	},
	{
		name:    "returns error",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(v struct{ Hello string }) error {
				assert.Equal(t, v.Hello, "hi")
				return errors.New("oh noz")
			})
		},
		expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"error":{"message":"oh noz"}}
		`,
	},
	{
		name:    "v->r,e",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(v struct{ Hello string }) (r string, e error) {
				return v.Hello, nil
			})
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":"hi"}
		`,
	},
	{
		name:    "v->r,e with error",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(v struct{ Hello string }) (r string, e error) {
				return v.Hello, errors.New("Oh noz.")
			})
		},
		expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"error":{"message":"Oh noz."}}
		`,
	},
	{
		name:    "v->r,e with input ptr",
		request: req("POST", "/", `{"hello":null}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(v *struct{ Hello string }) (r string, e error) {
				return v.Hello, nil
			})
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":""}
		`,
	},
	{
		name:    "w,r->",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(w http.ResponseWriter, r *http.Request) {
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
			return koi.New()(func(w http.ResponseWriter, r *http.Request) error {
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
			return koi.New()(func(w http.ResponseWriter, r *http.Request) error {
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

			{"error":{"message":"oh noz"}}
		`,
	},
	{
		name:    "w,r,v->",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) {
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
			return koi.New()(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
				assert.Equal(t, "hi", v.Hello)
				return nil
			})
		},
		expected: `
			HTTP/1.1 200 OK
			Connection: close
			Content-Type: application/json

			{"result":null}
		`,
	},
	{
		name:    "w,r,v->e error beats write",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) error {
				assert.Equal(t, "hi", v.Hello)
				w.Write([]byte("whaddup"))
				return errors.New("oh noz")
			})
		},
		expected: `
			HTTP/1.1 500 Internal Server Error
			Connection: close
			Content-Type: application/json

			{"error":{"message":"oh noz"}}
		`,
	},
	{
		name:    "w,r,v->r,v result beats write",
		request: req("POST", "/", `{"hello":"hi"}`),
		test: func(t *testing.T) http.HandlerFunc {
			return koi.New()(func(w http.ResponseWriter, r *http.Request, v struct{ Hello string }) (s struct{ Ok bool }, err error) {
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

			{"result":{"Ok":true}}
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
}

func TestKoi(t *testing.T) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := test.test(t)
			w := httptest.NewRecorder()
			fn(w, test.request)
			res := w.Result()
			buf, err := httputil.DumpResponse(res, true)
			assert.NoError(t, err)
			dif(t, test.expected, string(buf))
		})
	}
}

func TestExample(t *testing.T) {
	router := httprouter.New()
	handle := router.HandlerFunc
	koi := koi.New()

	type Input struct {
		Hello string `json:"hello,omitempty" validate:"required"`
	}

	type Output struct {
		Goodbye string `json:"goodbye,omitempty"`
	}

	handle("POST", "/", koi(func(v Input) (o Output, e error) {
		assert.Equal(t, v.Hello, "hola")
		o.Goodbye = "adios!"
		return o, nil
	}))

	r := req("POST", "/", `{"hello":"hola"}`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	res := w.Result()
	buf, err := httputil.DumpResponse(res, true)
	assert.NoError(t, err)
	dif(t, `
		HTTP/1.1 200 OK
		Connection: close
		Content-Type: application/json

		{"result":{"goodbye":"adios!"}}
	`, string(buf))
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
