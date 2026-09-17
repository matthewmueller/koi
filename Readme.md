# Koi

Koi is a typed request/response layer over `net/http`. Requires Go 1.27
(with generic methods).

```go
k := koi.New()

router := http.NewServeMux()
router.Handle("POST /boards", k.Create(createBoard).
    Redirect(func(board Board) string { return "/boards/" + board.ID }))

func createBoard(ctx context.Context, req *koi.Request[CreateBoard]) (res koi.Response[Board]) {
    board, err := saveBoard(ctx, req.Input)
    if err != nil {
        return res.Error(err)
    }
    return res.Body(board)
}
```

Controllers return `Response[Out]`. `Body`, `Render`, `Redirect`, `Error`, `Errorf`,
`Message`, `Status`, and `Header` return a modified copy; return or assign the result.
The zero response produces 204. `Body(nil)` still counts as a body.

## Routes

`k.Wrap(handler)` is the primitive. `Index`, `New`, `Show`, `Edit`, `Create`,
`Update`, and `Delete` use the same pipeline. Create defaults to 201; Delete
defaults to 204. Use `.View(name)`, `.Status(code)`, and `.Redirect(target)`
to configure routes. Redirect accepts a string or `func(Out) string`.
Response-level settings override route settings. Errors default to 500,
regardless of the route's success status.

Route redirects apply only to HTML success responses. A controller redirect
without a body applies to both formats; with a body, JSON receives the body.
Redirects use 303, independently of the normal response status.

## Input and validation

Input binds query parameters and JSON or URL-encoded form bodies using `json`
and `form` tags. Query and form decoding use `github.com/ajg/form`, which prefers
`form` tags over `json` tags. Duplicate query/body keys are rejected. Pointer
input is supported. `koi.Empty` represents no input. Configure an HTML error
view with `Route.ErrorView`; it receives a `ResponseError` containing the status,
safe message, and validation fields. JSON errors keep their structured response.

The router is responsible for adding path parameters to the request query
string. Koi does not read `PathValue` or interpret `path` or `query` tags.

The default validator handles `validate` tags and custom `Validate() koi.Invalid`
methods. Use `Invalid.Add` to produce safe, client-facing field errors.
Replace `k.Validator` to customize validation.
Validation field errors return 422 with an `error` and `fields` object in JSON.
Validators return `koi.Invalid`, or nil for valid input. Binding failures return 400.

## HTML and JSON

`Accept` quality values choose HTML or JSON. Missing headers and ties default
to JSON; unsupported representations return 406. `req.Accepts("html")` and
`req.Accepts("json")` check acceptability independently of the chosen format.
Responses vary on `Accept`. HEAD responses have no body.

HTML uses the route's view, overridden by `res.Render(name, value)`.
The default renderer uses `html/template` to parse and execute the filename
passed to `View` or `Render` on each request. View names must be trusted
application configuration. Rendering and JSON encoding finish before headers
are committed, so failures produce a clean 500 response.

Production errors expose only the HTTP status text. Set `k.Debug = true` to
expose error details. Logging uses a discard logger by default; assign `k.Log` a `*slog.Logger`
to record method, path, status, duration, and underlying errors. Configure Koi
and its routes before serving concurrent requests.

## Initial scope

This implementation covers ordinary request/response APIs. HTML validation
currently returns a safe 422 error page. Input persistence and flash-and-redirect
validation need a session/flash API and are deferred, along with webhooks with
raw byte input, SSE, WebSockets, streaming, and Raw handlers.
