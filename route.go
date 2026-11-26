package koi

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
)

// Route serves a typed controller as an http.Handler. Create routes through
// Koi.Wrap. Configuration methods modify and return the
// same route; configure it before serving concurrent requests.
type Route[Out any] struct {
	k         *Koi
	handle    func(http.ResponseWriter, *http.Request) Response[Out]
	view      string
	errorView string
	status    int
	redirect  string
	parent    parent
}

var _ http.Handler = (*Route[Empty])(nil)

// parent is a route whose view wraps another route's view. The interface
// hides the route's body type, so a route can have a parent of any type.
type parent interface {
	// load runs the route's handler and resolves its view.
	load(req *http.Request) Response[any]
	// above returns the route's own parent, or nil.
	above() parent
}

var _ parent = (*Route[Empty])(nil)

// View sets the default HTML template key. Response.Render overrides it.
// JSON responses do not use the view.
func (r *Route[Out]) View(key string) *Route[Out] {
	r.view = key
	return r
}

// ErrorView sets the HTML view used for errors on this route. If unset, Koi
// uses its built-in error page. JSON responses do not use the view.
func (r *Route[Out]) ErrorView(name string) *Route[Out] {
	r.errorView = name
	return r
}

// Status sets the default success status. Response.Status overrides it.
// Zero uses 200 for a body or 204 for an empty response. Errors and HTML
// redirects choose their own statuses.
func (r *Route[Out]) Status(code int) *Route[Out] {
	r.status = code
	return r
}

// Redirect configures a 303 redirect for successful HTML responses. A relative
// target is appended to the request path; "." redirects to the current URL;
// leading-slash and absolute targets are used directly. Response.Redirect
// overrides this default.
func (r *Route[Out]) Redirect(target string) *Route[Out] {
	r.redirect = target
	return r
}

// Parent renders this route's view inside the parent's view. For HTML
// requests, the parent's handler runs alongside this route's handler and its
// body becomes the props of its view. Its input is bound from the path and the
// query string, but not the body. If the parent fails or redirects, that
// replaces the page. A parent can have its own parent.
//
// JSON requests don't run parents. Only the parent's handler and view are
// used: its Status, Redirect and ErrorView are ignored.
func (r *Route[Out]) Parent[P any](parent *Route[P]) *Route[Out] {
	r.parent = parent
	return r
}

func (r *Route[Out]) above() parent {
	return r.parent
}

func (r *Route[Out]) load(req *http.Request) (out Response[any]) {
	// net/http only recovers panics in the goroutine it started, so a panic
	// here would otherwise end the process.
	defer func() {
		if v := recover(); v != nil {
			out = out.Errorf("koi: parent panicked: %v\n%s", v, debug.Stack())
		}
	}()
	// A parent has no body to limit, so there's no response writer to pass.
	res := r.handle(nil, req)
	// Like a page's body, an invalid one is a server bug. Errors and redirects
	// take precedence, as they do for a page.
	if res.err == nil && res.redirect == "" && res.hasBody {
		if invalid := r.k.Validator.Validate(&res.body); invalid != nil {
			return out.Errorf("koi: invalid response body: %v", invalid)
		}
	}
	res.view = cmp.Or(res.view, r.view)
	// A page reports its own missing view, so a parent that renders does too.
	if res.view == "" && res.err == nil && res.redirect == "" {
		return out.Errorf("koi: parent has no view; configure Route.View or Response.Render")
	}
	return Response[any]{body: res.body, outcome: res.outcome}
}

func logLevel(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status == 409:
		return slog.LevelWarn
	case status >= 400:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

func (r *Route[Out]) log(req *http.Request, status int, dur time.Duration, err error) {
	attrs := []slog.Attr{
		slog.String("method", req.Method),
		slog.String("path", req.URL.Path),
		slog.Int("status", status),
		slog.Duration("duration", dur),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	r.k.Log.LogAttrs(req.Context(), logLevel(status), "request", attrs...)
}

// ServeHTTP binds and validates input, invokes the controller, and writes the
// negotiated response. Missing Accept headers and equal preferences select JSON;
// requests accepting neither HTML nor JSON receive 406.
func (r *Route[Out]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	started := time.Now()
	status, err := r.serve(w, req)
	r.log(req, status, time.Since(started), err)
}

func (r *Route[Out]) serve(w http.ResponseWriter, req *http.Request) (status int, err error) {
	// Every negotiated response must be revalidated when a cache is involved.
	w.Header().Add("Vary", "Accept")

	// Reject unsupported representations before binding input or running the controller.
	asHTML, ok := negotiate(req.Header.Get("Accept"))
	if !ok {
		r.writeError(w, req, false, &ResponseError{
			Status:  http.StatusNotAcceptable,
			Message: http.StatusText(http.StatusNotAcceptable),
		})
		return http.StatusNotAcceptable, nil
	}

	// Parents load alongside the page. JSON doesn't render them, so it skips them.
	var loads []*Response[any]
	var wg sync.WaitGroup
	if asHTML && r.parent != nil {
		ctx, cancel := context.WithCancel(req.Context())
		defer wg.Wait() // no loader outlives ServeHTTP
		defer cancel()  // runs first, so any early return stops the loaders
		for p := r.parent; p != nil; p = p.above() {
			l := new(Response[any])
			loads = append(loads, l)
			// Parents bind from the path and query string. The body belongs to the page,
			// and a clone keeps a parent from sharing the page's headers and form.
			sub := req.Clone(ctx)
			sub.Body, sub.ContentLength = http.NoBody, 0
			wg.Go(func() { *l = p.load(sub) })
		}
	}

	res := r.handle(w, req)
	// Route redirects apply to HTML only.
	if res.redirect == "" && asHTML {
		res.redirect = r.redirect
	}
	if status := r.exit(w, req, asHTML, res.outcome); status != 0 {
		return status, res.err
	}

	// A parent's error or redirect replaces the page. The chain was walked
	// upward, so read it backward to check the outermost parent first.
	wg.Wait()
	for _, l := range slices.Backward(loads) {
		if status := r.exit(w, req, asHTML, l.outcome); status != 0 {
			return status, l.err
		}
	}
	// The page's headers go on last, so they win over its parents'.
	var views []View
	for _, l := range slices.Backward(loads) {
		mergeHeader(w.Header(), l.header)
		views = append(views, View{Name: l.view, Props: l.body})
	}
	mergeHeader(w.Header(), res.header)

	// Success status precedence: response override, route default, then body presence.
	status = res.status
	if status == 0 {
		status = r.status
	}
	if status == 0 {
		if res.hasBody {
			status = 200
		} else {
			status = 204
		}
	}

	// Skip rendering when there is no body or the status forbids one.
	if !res.hasBody || status == http.StatusNoContent || status == http.StatusNotModified {
		w.WriteHeader(status)
		return status, nil
	}

	// An invalid response body is a server bug, so it becomes a 500 rather than
	// a 422. Format with %v so the fields aren't treated as client input errors.
	if invalid := r.k.Validator.Validate(&res.body); invalid != nil {
		status = http.StatusInternalServerError
		err = fmt.Errorf("koi: invalid response body: %v", invalid)
		r.writeError(w, req, asHTML, r.k.errorResponse(status, err, ""))
		return status, err
	}

	// Render or encode before writing headers, so failures can still become 500.
	var buf bytes.Buffer
	view := res.view
	if view == "" {
		view = r.view
	}
	// Without a view, serve JSON to clients that also accept it, like browsers
	// that send "text/html, */*;q=0.8"
	if asHTML && view == "" && quality(req.Header.Get("Accept"), mediaType("json")) > 0 {
		asHTML = false
	}
	switch {
	case !asHTML:
		err = r.encodeJson(&buf, res.body)
	case view == "":
		err = errors.New("HTML response has no view; configure Route.View, Response.Render, or Route.Redirect")
	default:
		err = r.k.Renderer.Render(req.Context(), &buf, append(views, View{Name: view, Props: res.body})...)
	}
	if err != nil {
		status = http.StatusInternalServerError
		r.writeError(w, req, asHTML, r.k.errorResponse(status, err, ""))
		return status, err
	}

	if asHTML {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	return status, writeBody(w, req, status, buf.Bytes())
}

// exit writes the outcomes that end a request without rendering a body: an
// error or a redirect. A page and its parents share it. The response carries
// the headers of that outcome alone. A zero status means neither applied.
func (r *Route[Out]) exit(w http.ResponseWriter, req *http.Request, asHTML bool, res outcome) int {
	if res.err != nil {
		mergeHeader(w.Header(), res.header)
		// Errors use their explicit status, or 500 by default. Field errors
		// become a 422, so return the status that was written.
		page := r.k.errorResponse(cmp.Or(res.status, http.StatusInternalServerError), res.err, res.message)
		r.writeError(w, req, asHTML, page)
		return page.Status
	}
	// Redirects come after errors. One without a body also applies to JSON.
	if res.redirect != "" && (asHTML || !res.hasBody) {
		mergeHeader(w.Header(), res.header)
		http.Redirect(w, req, resolveRedirect(req, res.redirect), http.StatusSeeOther)
		return http.StatusSeeOther
	}
	return 0
}

// mergeHeader copies src into dst. Vary values are appended instead, so they
// add to "Vary: Accept".
func mergeHeader(dst, src http.Header) {
	for key, values := range src {
		if key == "Vary" {
			for _, value := range values {
				dst.Add(key, value)
			}
		} else {
			dst[key] = values
		}
	}
}

func resolveRedirect(req *http.Request, target string) string {
	if target == "." {
		return req.URL.RequestURI()
	}
	if target == "" || strings.HasPrefix(target, "/") {
		return target
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return target
	}
	u := *req.URL
	u.Path = path.Join(strings.TrimRight(u.Path, "/"), strings.TrimLeft(parsed.Path, "/"))
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	if parsed.RawQuery != "" {
		u.RawQuery = parsed.RawQuery
	}
	if parsed.Fragment != "" {
		u.Fragment = parsed.Fragment
	}
	return u.String()
}

func negotiate(accept string) (bool, bool) {
	htmlQ, jsonQ := quality(accept, "text/html"), quality(accept, "application/json")
	return htmlQ > jsonQ, htmlQ > 0 || jsonQ > 0
}

func invalidFields(err error) (Invalid, bool) {
	if fields, ok := errors.AsType[Invalid](err); ok {
		return fields, true
	}
	if fields, ok := errors.AsType[*Invalid](err); ok && fields != nil {
		return *fields, true
	}
	return nil, false
}

type jsonError struct {
	Error  string              `json:"error"`
	Fields map[string][]string `json:"fields,omitempty"`
}

// errorResponse resolves the public message and validation fields once.
func (k *Koi) errorResponse(status int, err error, message string) *ResponseError {
	fields, invalid := invalidFields(err)
	// Explicit messages take precedence. Validation text is public; other error
	// details are exposed only in debug mode.
	if message == "" {
		switch {
		case invalid:
			message = fields.Error()
			status = http.StatusUnprocessableEntity
		case k.Debug && err != nil:
			message = err.Error()
		default:
			message = http.StatusText(status)
		}
	}
	return &ResponseError{
		Status:  status,
		Message: message,
		Fields:  fields,
	}
}

//go:embed error.gohtml
var errorPage string

var errorTemplate = template.Must(template.New("error.gohtml").Parse(errorPage))

func (r *Route[Out]) writeError(w http.ResponseWriter, req *http.Request, asHTML bool, resErr *ResponseError) {
	// An error replaces any previously planned body, so its length no longer applies.
	w.Header().Del("Content-Length")
	var body bytes.Buffer
	if asHTML {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Buffer custom views so a failed render can fall back without leaking output.
		if r.errorView != "" {
			if err := r.k.Renderer.Render(req.Context(), &body, View{Name: r.errorView, Props: resErr}); err == nil {
				writeBody(w, req, resErr.Status, body.Bytes())
				return
			}
			// Discard partial output before writing the built-in error page.
			body.Reset()
		}
		if err := errorTemplate.Execute(&body, resErr); err != nil {
			body.Reset()
			body.WriteString("Koi error page failed to render")
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		// This fixed string/map payload and bytes.Buffer cannot cause encoding errors.
		r.encodeJson(&body, jsonError{
			resErr.Message,
			resErr.Fields,
		})
	}
	writeBody(w, req, resErr.Status, body.Bytes())
}

func writeBody(w http.ResponseWriter, r *http.Request, status int, body []byte) error {
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return nil
	}
	_, err := w.Write(body)
	return err
}

func (r *Route[Out]) encodeJson(w io.Writer, v any) error {
	var opts []json.Options
	if r.k.Debug {
		opts = append(opts, jsontext.WithIndent("\t"))
	}
	return json.MarshalEncode(jsontext.NewEncoder(w, opts...), v)
}
