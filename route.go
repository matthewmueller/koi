package koi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/matthewmueller/koi/internal/request"
)

// Route serves a typed controller as an http.Handler. Create routes through
// Koi.Wrap. Configuration methods modify and return the
// same route; configure it before serving concurrent requests.
type Route[Out any] struct {
	k         *Koi
	handle    func(http.ResponseWriter, *http.Request) (Response[Out], error)
	view      string
	errorView string
	status    int
	redirect  string
}

var _ http.Handler = (*Route[Empty])(nil)

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
	res, err := r.handle(w, req)

	if err != nil {
		// Binding and validation happen inside the route handler wrapper.
		page := r.k.errorResponse(toErrorStatus(err), err, "")
		r.writeError(w, req, asHTML, page)
		return page.Status, err
	}

	// Send headers after both successful responses and controller errors.
	headers := w.Header()
	for key, values := range res.header {
		if key == "Vary" {
			for _, value := range values {
				headers.Add(key, value)
			}
		} else {
			headers[key] = values
		}
	}

	if res.err != nil {
		// Controller errors use their explicit status, or 500 by default.
		status := res.status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		r.writeError(w, req, asHTML, r.k.errorResponse(status, res.err, res.message))
		return status, res.err
	}

	// Resolve redirects only after ruling out errors. Route redirects apply to HTML;
	// an explicit response redirect without a body also applies to JSON.
	status = res.status
	target := res.redirect
	if target == "" && asHTML {
		target = r.redirect
	}
	if target != "" && (asHTML || !res.hasBody) {
		target = resolveRedirect(req, target)
		status = http.StatusSeeOther
		http.Redirect(w, req, target, status)
		return status, nil
	}

	// Success status precedence: response override, route default, then body presence.
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

	// Render or encode before writing headers, so failures can still become 500.
	var buf bytes.Buffer
	if asHTML {
		view := res.view
		if view == "" {
			view = r.view
		}
		if view == "" {
			err = errors.New("HTML response has no view; configure Route.View, Response.Render, or Route.Redirect")
		} else {
			err = r.k.Renderer.Render(req.Context(), &buf, view, res.body)
		}
	} else {
		err = r.encodeJson(&buf, res.body)
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

// toErrorStatus maps an input error to its client error status.
func toErrorStatus(err error) int {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, request.ErrUnsupportedMediaType) {
		return http.StatusUnsupportedMediaType
	}
	return http.StatusBadRequest
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
			if err := r.k.Renderer.Render(req.Context(), &body, r.errorView, resErr); err == nil {
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
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if r.k.Debug {
		enc.SetIndent("", "\t")
	}
	return enc.Encode(v)
}
