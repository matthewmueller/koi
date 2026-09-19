package request_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajg/form"
	"github.com/matryer/is"
	"github.com/matthewmueller/koi/internal/request"
)

func TestUnsupportedMediaType(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=matt"))
	r.Header.Set("Content-Type", "text/plain")

	err := request.Unmarshal(r, &input)

	is.True(errors.Is(err, request.ErrUnsupportedMediaType))
}

func TestMissingContentType(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"matt"}`))

	err := request.Unmarshal(r, &input)

	is.True(errors.Is(err, request.ErrUnsupportedMediaType))
}

func TestFormParseError(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=%"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err := request.Unmarshal(r, &input)

	is.True(err != nil)
}

func TestJSONMultipleValues(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"matt"}{"name":"matthew"}`))
	r.Header.Set("Content-Type", "application/json")

	err := request.Unmarshal(r, &input)

	is.Equal(err.Error(), "request body must contain a single JSON value")
}

func TestJSONTrailingData(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"matt"} nope`))
	r.Header.Set("Content-Type", "application/json")

	err := request.Unmarshal(r, &input)

	is.True(err != nil)
}

func TestDuplicateJSONQueryField(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/?name=query", strings.NewReader(`{"name":"body"}`))
	r.Header.Set("Content-Type", "application/json")

	err := request.Unmarshal(r, &input)

	is.True(errors.Is(err, request.ErrDuplicateField))
}

func TestDuplicateFormQueryField(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/?name=query", strings.NewReader("name=body"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err := request.Unmarshal(r, &input)

	is.True(errors.Is(err, request.ErrDuplicateField))
}

func TestJSONSuffixMediaType(t *testing.T) {
	is := is.New(t)
	type Input struct{ Name string }
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"matt"}`))
	r.Header.Set("Content-Type", "application/activity+json")

	err := request.Unmarshal(r, &input)

	is.NoErr(err)
	is.Equal(input.Name, "matt")
}

func TestJSONEmpty(t *testing.T) {
	is := is.New(t)
	type S struct{}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
}

func TestFormEmpty(t *testing.T) {
	is := is.New(t)
	type S struct{}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
}

func TestStringJSONEmpties(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("", s.A)
	is.Equal("", s.B)
}

func TestStringFormEmpties(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("", s.A)
	is.Equal("", s.B)
}

func TestStringJSONEmptyQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=a&b=b", nil)
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
}

func TestStringFormEmptyQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=a&b=b", nil)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
}

func TestStringJSONQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
		C string
	}
	s := S{}
	r := httptest.NewRequest("POST", "/?a=a&b=b", bytes.NewBufferString(`{"c":"c"}`))
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
	is.Equal("c", s.C)
}

func TestStringFormQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
		C string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=a&b=b", bytes.NewBufferString(`c=c`))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
	is.Equal("", s.C)
}

// func TestStringJSONQueryExtra(t *testing.T) {
// is := is.New(t)
// 	type S struct {
// 		A string
// 		B string
// 		C string
// 		D string
// 	}
// 	s := S{}
// 	r := httptest.NewRequest("GET", "/?a=a&b=b", bytes.NewBufferString(`{"c":"c"}`))
// 	r.Header.Add("Content-Type", "application/json")
// 	err := request.Unmarshal(r, &s)
// 	is.NoErr(err)
// 	is.Equal( "a", s.A)
// 	is.Equal( "b", s.B)
// 	is.Equal( "", s.C)
// 	is.Equal( "", s.D)
// }

// func TestStringFormQueryExtra(t *testing.T) {
// is := is.New(t)
// 	type S struct {
// 		A string
// 		B string
// 		C string
// 		D string
// 	}
// 	s := S{}
// 	r := httptest.NewRequest("GET", "/?a=a&b=b", bytes.NewBufferString(`c=c`))
// 	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
// 	err := request.Unmarshal(r, &s)
// 	is.NoErr(err)
// 	is.Equal( "a", s.A)
// 	is.Equal( "b", s.B)
// 	is.Equal( "c", s.C)
// 	is.Equal( "", s.D)
// }

func TestStringJSONQueryOverride(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=a&b=b", bytes.NewBufferString(`{"b":"c"}`))
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
}

func TestStringFormQueryOverride(t *testing.T) {
	is := is.New(t)
	type S struct {
		A string
		B string
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=a&b=b", bytes.NewBufferString(`b=c`))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal("a", s.A)
	is.Equal("b", s.B)
}

func TestNumberJSONEmpties(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(0, s.A)
	is.Equal(0.0, s.B)
}

func TestNumberFormEmpties(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(0, s.A)
	is.Equal(0.0, s.B)
}

func TestNumberJSONEmptyQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=20&b=10.2", nil)
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(20, s.A)
	is.Equal(10.2, s.B)
}

func TestNumberFormEmptyQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=1&b=2.2", nil)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.A)
	is.Equal(2.2, s.B)
}

func TestNumberJSONQuery(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
		C int
		D float64
	}
	s := S{}
	r := httptest.NewRequest("POST", "/?a=1&b=2.2", bytes.NewBufferString(`{"c":3,"d":4.4}`))
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.A)
	is.Equal(2.2, s.B)
	is.Equal(3, s.C)
	is.Equal(4.4, s.D)
}

// func TestNumberFormQuery(t *testing.T) {
// is := is.New(t)
// 	type S struct {
// 		A int
// 		B float64
// 		C int
// 		D float64
// 	}
// 	s := S{}
// 	r := httptest.NewRequest("GET", "/?a=1&b=2.2", bytes.NewBufferString(`c=3&d=4.4`))
// 	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
// 	err := request.Unmarshal(r, &s)
// 	is.NoErr(err)
// 	is.Equal( 1, s.A)
// 	is.Equal( 2.2, s.B)
// 	is.Equal( 3, s.C)
// 	is.Equal( 4.4, s.D)
// }

// func TestNumberJSONQueryExtra(t *testing.T) {
// is := is.New(t)
// 	type S struct {
// 		A int
// 		B float64
// 		C int
// 		D float64
// 	}
// 	s := S{}
// 	r := httptest.NewRequest("GET", "/?a=1&b=2.2", bytes.NewBufferString(`{"c":3}`))
// 	r.Header.Add("Content-Type", "application/json")
// 	err := request.Unmarshal(r, &s)
// 	is.NoErr(err)
// 	is.Equal( 1, s.A)
// 	is.Equal( 2.2, s.B)
// 	is.Equal( 0, s.C)
// 	is.Equal( 0.0, s.D)
// }

// func TestNumberFormQueryExtra(t *testing.T) {
// is := is.New(t)
// 	type S struct {
// 		A int
// 		B float64
// 		C int
// 		D float64
// 	}
// 	s := S{}
// 	r := httptest.NewRequest("GET", "/?a=1&b=2.2", bytes.NewBufferString(`c=3`))
// 	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
// 	err := request.Unmarshal(r, &s)
// 	is.NoErr(err)
// 	is.Equal( 1, s.A)
// 	is.Equal( 2.2, s.B)
// 	is.Equal( 3, s.C)
// 	is.Equal( 0.0, s.D)
// }

func TestNumberJSONQueryOverride(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=1&b=2.2", bytes.NewBufferString(`{"b":3.3}`))
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.A)
	is.Equal(2.2, s.B)
}

func TestNumberFormQueryOverride(t *testing.T) {
	is := is.New(t)
	type S struct {
		A int
		B float64
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?a=1&b=2.2", bytes.NewBufferString(`b=3.3`))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.A)
	is.Equal(2.2, s.B)
}

func TestJSONKey(t *testing.T) {
	is := is.New(t)
	type S struct {
		PostID int `json:"post_id"`
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?post_id=1", nil)
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.PostID)
}

func TestFormKey(t *testing.T) {
	is := is.New(t)
	type S struct {
		PostID int `form:"post_id" json:"post_ids"`
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?post_id=1", nil)
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(1, s.PostID)
}

func TestJSONGet(t *testing.T) {
	is := is.New(t)
	type S struct {
		PostID int    `json:"post_id"`
		Order  string `json:"order"`
		Author string `json:"author"`
	}
	s := S{}
	r := httptest.NewRequest("GET", "/?post_id=10&order=asc&author=Alice", nil)
	err := request.Unmarshal(r, &s)
	is.NoErr(err)
	is.Equal(10, s.PostID)
	is.Equal("asc", s.Order)
	is.Equal("Alice", s.Author)
}

func TestFormSlice(t *testing.T) {
	is := is.New(t)
	query := new(bytes.Buffer)
	type S struct {
		Categories []string `json:"categories"`
	}
	s := S{
		Categories: []string{"a", "b", "c"},
	}
	is.NoErr(form.NewEncoder(query).Encode(s))
	is.Equal(query.String(), `categories.0=a&categories.1=b&categories.2=c`)
	s2 := S{}
	r := httptest.NewRequest("GET", "/?"+query.String(), nil)
	err := request.Unmarshal(r, &s2)
	is.NoErr(err)
	is.Equal(s.Categories, s2.Categories)
}

func TestFormMap(t *testing.T) {
	is := is.New(t)
	query := new(bytes.Buffer)
	type S struct {
		Categories map[string]int `json:"categories"`
	}
	s := S{
		Categories: map[string]int{
			"a":   1,
			"b":   2,
			"c.d": 3,
		},
	}
	is.NoErr(form.NewEncoder(query).Encode(s))
	is.Equal(query.String(), `categories.a=1&categories.b=2&categories.c%5C.d=3`)
	s2 := S{}
	r := httptest.NewRequest("GET", "/?"+query.String(), nil)
	err := request.Unmarshal(r, &s2)
	is.NoErr(err)
	is.Equal(s.Categories, s2.Categories)
}

func TestFormMapBody(t *testing.T) {
	is := is.New(t)
	type Input struct {
		Categories map[string]int `form:"categories"`
	}
	input := Input{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("categories.a=1&categories.b=2&categories.c%5C.d=3"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err := request.Unmarshal(r, &input)

	is.NoErr(err)
	is.Equal(input.Categories, map[string]int{"a": 1, "b": 2, "c.d": 3})
}

func TestPointer(t *testing.T) {
	is := is.New(t)
	type Input struct {
		Author *string `json:"author"`
	}
	var in Input
	r := httptest.NewRequest("GET", "/?author=Alice", nil)
	err := request.Unmarshal(r, &in)
	is.NoErr(err)
	is.True(in.Author != nil)
	is.Equal("Alice", *in.Author)
	in = Input{}
	r = httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"author":"Alice"}`))
	r.Header.Add("Content-Type", "application/json")
	err = request.Unmarshal(r, &in)
	is.NoErr(err)
	is.True(in.Author != nil)
	is.Equal("Alice", *in.Author)
	// Test optional
	in = Input{}
	r = httptest.NewRequest("POST", "/", nil)
	err = request.Unmarshal(r, &in)
	is.NoErr(err)
	is.Equal(nil, in.Author)
}

func TestNestedInput(t *testing.T) {
	is := is.New(t)
	type Param struct {
		Version int
		Update  bool `json:"update"`
	}
	type Op struct {
		Name   string   `json:"name"`
		Params []*Param `json:"params"`
	}
	type Name string
	type Input struct {
		ID   int  `json:"id"`
		Name Name `json:"name"`
		Op   *Op  `json:"op"`
	}
	in := Input{}
	r := httptest.NewRequest("POST", "/?id=123&name=create", bytes.NewBufferString(`{
		"op": {
			"name": "update",
			"params": [
				{ "Version": 1, "update": true },
				{ "Version": 2 }
			]
		}
	}`))
	r.Header.Add("Content-Type", "application/json")
	err := request.Unmarshal(r, &in)
	is.NoErr(err)
	is.Equal(123, in.ID)
	is.Equal(Name("create"), in.Name)
	is.Equal("update", in.Op.Name)
	is.Equal(2, len(in.Op.Params))
	is.Equal(1, in.Op.Params[0].Version)
	is.Equal(true, in.Op.Params[0].Update)
	is.Equal(2, in.Op.Params[1].Version)
	is.Equal(false, in.Op.Params[1].Update)
}

func TestUnmarshalSources(t *testing.T) {
	is := is.New(t)
	type Input struct {
		ID    int    `json:"id"`
		Limit int    `json:"per_page"`
		Name  string `json:"name"`
	}
	var input Input
	r := httptest.NewRequest(http.MethodPost, "/?id=42&per_page=25", strings.NewReader(`{"name":"Alice"}`))
	r.Header.Set("Content-Type", "application/json")
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
	is.Equal(input.ID, 42)
	is.Equal(input.Limit, 25)
	is.Equal(input.Name, "Alice")
}

func TestUnmarshalPointerForm(t *testing.T) {
	is := is.New(t)
	type Input struct {
		ID   string `json:"id"`
		Page int    `form:"page_number" json:"page"`
		Name string `form:"display_name"`
	}
	var input *Input
	r := httptest.NewRequest(http.MethodPost, "/?id=board-1&page_number=2", strings.NewReader("display_name=Alice"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
	is.Equal(input.ID, "board-1")
	is.Equal(input.Page, 2)
	is.Equal(input.Name, "Alice")
}

func TestUnmarshalInvalidQuery(t *testing.T) {
	is := is.New(t)
	var input struct {
		ID int `json:"id"`
	}
	r := httptest.NewRequest(http.MethodGet, "/?id=invalid", nil)
	err := request.Unmarshal(r, &input)
	is.True(err != nil)
}

func TestUnmarshalDuplicate(t *testing.T) {
	is := is.New(t)
	var input struct{ Name string }
	r := httptest.NewRequest(http.MethodPost, "/?name=query", strings.NewReader(`{"name":"body"}`))
	r.Header.Set("Content-Type", "application/json")
	err := request.Unmarshal(r, &input)
	is.True(errors.Is(err, request.ErrDuplicateField))
}

func TestUnmarshalEmpty(t *testing.T) {
	is := is.New(t)
	var input struct{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
}

func TestUnmarshalQueryIgnored(t *testing.T) {
	is := is.New(t)
	var input struct {
		Name string `form:"-"`
	}
	r := httptest.NewRequest(http.MethodGet, "/?name=Alice", nil)
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
	is.Equal(input.Name, "")
}

func TestUnmarshalFormTagOverridesJSON(t *testing.T) {
	is := is.New(t)
	var input struct {
		Page int `form:"page" json:"-"`
	}
	r := httptest.NewRequest(http.MethodGet, "/?page=3", nil)
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
	is.Equal(input.Page, 3)
}

func TestUnmarshalQueryPointer(t *testing.T) {
	is := is.New(t)
	var input struct {
		ID *int `json:"id"`
	}
	r := httptest.NewRequest(http.MethodGet, "/?id=42", nil)
	err := request.Unmarshal(r, &input)
	is.NoErr(err)
	is.True(input.ID != nil)
	is.Equal(*input.ID, 42)
}
