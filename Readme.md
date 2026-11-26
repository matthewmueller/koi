# Koi

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/koi.svg)](https://pkg.go.dev/github.com/matthewmueller/koi)

A minimal, type-safe request/response layer for Go. Controllers choose an outcome;
Koi handles binding, validation, and HTML/JSON responses.

## Features

- Typed request inputs and response bodies
- Automatic path, query, form, and JSON binding
- Validation with client-facing field errors
- HTML/JSON negotiation using the request's `Accept` header
- Layouts that load their own data alongside the page
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

func greet(req *koi.Request[GreetingInput]) (res koi.Response[Greeting]) {
	return res.Body(Greeting{Message: "Hello, " + req.Input.Name + "!"})
}

func main() {
	k := koi.New()

	router := http.NewServeMux()
	router.Handle("GET /hello", k.Wrap(greet).View("hello.html"))

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

Use `k.Wrap(handler)` for any typed handler. Set an explicit status when a
route needs one, then add a view or redirect as needed.

```go
	k.Wrap(createBoard).
	Status(http.StatusCreated).
	Redirect("10")
```

Relative redirect targets append to the current path, so a request to
`/boards` with `Redirect("10")` redirects HTML to `/boards/10`. A target
of `"."` redirects to the current URL, and `".."` redirects to its parent
path. Other relative paths such as `"../some/path"` are cleaned and appended
to the current path. A target beginning with `/` or an absolute `http://` or
`https://` URL is used directly. Successful HTML requests redirect with 303;
JSON requests receive the response body when one is present. Use `koi.Empty`
for handlers with no input or output data.

Wildcards in a `net/http` pattern bind to the input by name. They win over a
query value of the same name, and a body field of the same name returns 400:

```go
type ShowInput struct {
	ID string `json:"id"`
}

router.Handle("GET /boards/{id}", k.Wrap(show))
```

Routers that put path parameters in the query string work too.

## Parents

A route can render inside another route's view. The parent has its own handler,
which loads the data for its view while the page's handler runs:

```go
type NavInput struct {
	Tab string `form:"tab"`
}

type Nav struct {
	Tab string
}

func loadNav(req *koi.Request[NavInput]) (res koi.Response[Nav]) {
	return res.Body(Nav{Tab: req.Input.Tab})
}

layout := k.Wrap(loadNav).View("layout.html")

router.Handle("GET /hello", k.Wrap(greet).View("hello.html").Parent(layout))
router.Handle("GET /bye", k.Wrap(bye).View("bye.html").Parent(layout))
```

With the default renderer, the parent's template places the page with `slot`:

```html
<nav>{{.Tab}}</nav>
<main>{{slot}}</main>
```

- **Parents only run for HTML requests.** JSON requests get the page's data
  alone, so check access in middleware or the page's handler, not in a parent.
- **A parent binds its input from the path and the query string.** The request
  body belongs to the page.
- **A parent's error or redirect replaces the page.** `res.Status(404).Error(err)`
  in a parent sends the 404 error page.
- **A parent can have a parent**, like `shell.Parent(layout)`. Views render
  outermost first.
- **The page's headers win** when a parent sets the same one. An error or
  redirect carries only the headers of the route that returned it.
- **Only the parent's handler and view are used.** Its `Status`, `Redirect` and
  `ErrorView` are ignored.

## Validation and errors

Use `validate` tags, implement `Validate() koi.Invalid` on your input, or both.
Field errors from both are merged and named by the `form` tag, then `json`:

```go
func (in *CreateBoard) Validate() koi.Invalid {
	var invalid koi.Invalid
	if in.Name == "" {
		invalid.Add("name", "is required")
	}
	return invalid
}
```

Validation failures return 422 with field errors. Unsupported content types
return 415, and bodies over `k.MaxBodySize` return 413. HTML currently shows an error
page; flash-and-redirect validation is not yet supported.

A JSON value of the wrong type is reported the same way, under the name the
client sent:

```sh
curl -X PUT -H 'Content-Type: application/json' -d '{"networking": {"type": 5}}' localhost:8080/environments/dev
# 422 {"error":"networking.type can't be a number","fields":{"networking.type":["can't be a number"]}}
```

Fields are named by their path, like `networking.type` or `tags.1`, in both
decoding and validation errors. An embedded struct's fields are named as its
parent's.

Malformed JSON and a few other errors that only describe the request, like a
bad URL escape, return 400 and say what's wrong. Everything else returns its
status text unless `k.Debug` is on, because the error could describe the
server: a body that fails to read names the server's address, and a form value
of the wrong type prints the input's Go type.

Response bodies are validated the same way before they're sent. An invalid
response body is a server bug, so it returns 500 and its field errors are
only exposed with `k.Debug`.

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
`k.Log` to enable request logging, including underlying errors. Set
`k.MaxBodySize` to change the 10MB request body limit, or 0 to disable it.

JSON bodies with fields the input doesn't declare are rejected, so a misspelled
`{"netwrking": {}}` returns 422 instead of being silently dropped. Set
`k.AllowUnknownFields = true` to ignore them. Query strings and forms always
ignore unknown keys.

## Contributions

Pull requests, bug reports, and feature requests are welcome!

## API reference

### Handlers and requests

```go
type Handler[In, Out any] func(*Request[In]) Response[Out]
```

`Request[In]` embeds `*http.Request` and provides decoded input through `Input`.
`req.Accepts("html")` and `req.Accepts("json")` check accepted representations.
Missing `Accept` headers and equal preferences select JSON. Responses without a
view are also sent as JSON when the client accepts it, so browsers can read JSON
routes.

### Routes

`koi.New()` creates a configured `*Koi`. `k.Wrap(handler)` returns a
`*Route[Out]` implementing `http.Handler`.

| Method | Behavior |
| --- | --- |
| `View(name)` | Set the default HTML template filename. |
| `Parent(route)` | Render inside the route's view, with data from its handler. |
| `ErrorView(name)` | Set the HTML error template; failed rendering uses the built-in page. |
| `Status(code)` | Set the default success status. |
| `Redirect(target)` | Redirect successful HTML responses; relative targets append to the current path. |

Route methods modify the route. Configure routes before serving requests.

### Responses

Response methods return a copy; return or assign their result.

| Method | Behavior |
| --- | --- |
| `Body(value)` | Supply typed data for JSON encoding or HTML rendering. |
| `Render(name, value)` | Supply data and override the HTML template. |
| `Redirect(target)` | Redirect both formats without a body, or HTML only with a body; relative targets append to the current path, support `.`/`..` segments, and absolute URLs pass through unchanged. |
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

type View = struct {
	Name  string
	Props any
}

type Renderer interface {
	Render(ctx context.Context, w io.Writer, views ...View) error
}

type ResponseError struct {
	Status  int
	Message string
	Fields  Invalid
}
```

Replace `k.Validator` or `k.Renderer` to implement these interfaces. Custom error
templates receive `*ResponseError`; ordinary templates receive the response body.

`Render` receives the page's parents, outermost first, followed by the page.
`View` is an alias of an unnamed struct, so a renderer can declare the same type
and implement `Renderer` without importing Koi.
