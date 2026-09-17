# Koi

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/koi.svg)](https://pkg.go.dev/github.com/matthewmueller/koi)

A minimal, type-safe request/response layer for Go. Controllers choose an outcome;
Koi handles binding, validation, and HTML/JSON responses.

## Features

- Typed request inputs and response bodies
- Automatic query, form, and JSON binding
- Validation with client-facing field errors
- HTML/JSON negotiation using the request's `Accept` header
- Pluggable rendering and validation
- Works with `net/http` routers and middleware

## Install

Requires Go 1.27+ with generic methods.

```sh
go get github.com/matthewmueller/koi
```

## Example

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/matthewmueller/koi"
)

type GreetingInput struct {
	Name string `form:"name" validate:"required"`
}

type Greeting struct {
	Message string `json:"message"`
}

func greet(ctx context.Context, req *koi.Request[GreetingInput]) (res koi.Response[Greeting]) {
	return res.Body(Greeting{Message: "Hello, " + req.Input.Name + "!"})
}

func main() {
	k := koi.New()

	router := http.NewServeMux()
	router.Handle("GET /hello", k.Show(greet).View("hello.html"))

	log.Fatal(http.ListenAndServe(":8080", router))
}
```

Create `hello.html`:

```html
<h1>{{.Message}}</h1>
```

Visit `/hello?name=Alice` in a browser for HTML, or request JSON:

```sh
curl -H 'Accept: application/json' 'localhost:8080/hello?name=Alice'
# {"message":"Hello, Alice!"}
```

## Routes

Use `k.Wrap(handler)` for any typed handler, or the resource presets:
`Index`, `New`, `Show`, `Edit`, `Create`, `Update`, and `Delete`.
Create defaults to 201; Delete defaults to 204.

```go
k.Create(createBoard).
	Redirect(func(board Board) string {
		return "/boards/" + board.ID
	})
```

Successful HTML requests redirect with 303; JSON requests receive the response
body. Use `koi.Empty` for handlers with no input or output data.

## Validation and errors

Use `validate` tags or implement `Validate() koi.Invalid` on your input:

```go
func (in *CreateBoard) Validate() koi.Invalid {
	var invalid koi.Invalid
	if in.Name == "" {
		invalid.Add("name", "is required")
	}
	return invalid
}
```

Validation failures return 422 with field errors. HTML currently shows an error
page; flash-and-redirect validation is not yet supported.

```go
return res.Status(404).Errorf("board %s was not found", id)
```

Error details are hidden in production. Use `res.Message("Board not found")` for
safe public text, or `k.Debug = true` to expose underlying errors during development.

Configure `Route.ErrorView("error.html")` for a custom HTML error page. Its template
receives `Status`, `Message`, and `Fields`; otherwise Koi supplies a basic page.

## Configuration

Replace `k.Renderer` or `k.Validator` to customize rendering and validation.
The default renderer uses `html/template` files. Assign a `*slog.Logger` to
`k.Log` to enable request logging, including underlying errors.

## Contributions

Pull requests, bug reports, and feature requests are welcome!

## API reference

### Handlers and requests

```go
type Handler[In, Out any] func(context.Context, *Request[In]) Response[Out]
```

`Request[In]` embeds `*http.Request` and provides decoded input through `Input`.
`req.Accepts("html")` and `req.Accepts("json")` check accepted representations.
Missing `Accept` headers and equal preferences select JSON.

### Routes

`koi.New()` creates a configured `*Koi`. `k.Wrap(handler)` and each resource
preset return a `*Route[Out]` implementing `http.Handler`.

| Method | Behavior |
| --- | --- |
| `View(name)` | Set the default HTML template filename. |
| `ErrorView(name)` | Set the HTML error template; failed rendering uses the built-in page. |
| `Status(code)` | Set the default success status. |
| `Redirect(target)` | Redirect successful HTML responses; accepts a string or `func(Out) string`. |

Route methods modify the route. Configure routes before serving requests.

### Responses

Response methods return a copy; return or assign their result.

| Method | Behavior |
| --- | --- |
| `Body(value)` | Supply typed data for JSON encoding or HTML rendering. |
| `Render(name, value)` | Supply data and override the HTML template. |
| `Redirect(target)` | Redirect both formats without a body, or HTML only with a body. |
| `Error(err)` | Set a failure, defaulting to 500; nil clears it. |
| `Errorf(format, args...)` | Set a formatted error with `fmt.Errorf`, including `%w` wrapping. |
| `Message(format, args...)` | Set safe public error text using `fmt.Sprintf`. |
| `Status(code)` | Override the success or error status. Redirects still use 303. |
| `Header(name, value)` | Set a response header. |

An empty successful response normally returns 204. Errors take precedence over
bodies and redirects. `Message` changes the text of a failure; use `Error` or
`Errorf` to set the failure itself.

### Validation and rendering

`Invalid` is a `map[string][]string`. Its pointer method `Add(field, message)`
initializes a nil map and appends a message. `Error()` produces a readable summary;
`MarshalJSON()` encodes the summary and field messages.

```go
type Validator interface {
	Validate(any) Invalid
}

type Renderer interface {
	Render(context.Context, io.Writer, string, any) error
}

type ResponseError struct {
	Status  int
	Message string
	Fields  Invalid
}
```

Replace `k.Validator` or `k.Renderer` to implement these interfaces. Custom error
templates receive `*ResponseError`; ordinary templates receive the response body.
