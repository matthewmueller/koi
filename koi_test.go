package koi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/koi"
)

type board struct {
	ID string `json:"id"`
}

func boardHandler(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
	return res.Body(board{ID: "42"})
}

func serve(t testing.TB, h http.Handler, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.Header.Set("Accept", accept)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestResourcePresets(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	w := serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Index(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.New(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Show(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Edit(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Create(boardHandler), "application/json")
	is.Equal(w.Code, 201)
	w = serve(t, k.Update(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Delete(boardHandler), "application/json")
	is.Equal(w.Code, 204)
}

func TestStatusOverrides(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Create(boardHandler).Status(http.StatusAccepted).Redirect(func(b board) string { return "/boards/" + b.ID })
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 202)
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
	w = serve(t, route, "text/html")
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/boards/42")
	route = k.Create(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(203).Header("X-Board", "42").Body(board{ID: "42"})
	}).Status(202)
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 203)
	is.Equal(w.Header().Get("X-Board"), "42")
}

func TestRepresentations(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	view := writeView(t, `<h1>{{.ID}}</h1>`)
	custom := writeView(t, `<p>{{.ID}}</p>`)
	route := k.Show(boardHandler).View(view)
	w := serve(t, route, "text/html")
	is.Equal(w.Body.String(), "<h1>42</h1>")
	is.Equal(w.Header().Get("Content-Type"), "text/html; charset=utf-8")
	w = serve(t, route, "application/json")
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
	is.Equal(w.Header().Get("Vary"), "Accept")
	route = k.Show(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Render(custom, board{ID: "42"})
	}).View(view)
	w = serve(t, route, "text/html")
	is.Equal(w.Body.String(), "<p>42</p>")
	w = serve(t, route, "application/json")
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
}

func TestRedirects(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Redirect("/login")
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 303)
	w = serve(t, route, "text/html")
	is.Equal(w.Header().Get("Location"), "/login")
	route = k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Redirect("/chosen").Body(board{ID: "42"})
	}).Redirect("/default")
	w = serve(t, route, "text/html")
	is.Equal(w.Header().Get("Location"), "/chosen")
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 200)
	empty := func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[koi.Empty]) { return res }
	deletion := k.Delete(empty).Redirect("/boards")
	w = serve(t, deletion, "text/html")
	is.Equal(w.Code, 303)
	w = serve(t, deletion, "application/json")
	is.Equal(w.Code, 204)
	is.Equal(w.Body.Len(), 0)
	w = serve(t, k.Wrap(empty), "application/json")
	is.Equal(w.Code, 204)
}

func TestErrorPrivacyAndLogging(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	route := k.Create(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Error(errors.New("postgres: secret password"))
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
	is.True(!strings.Contains(serve(t, route, "text/html").Body.String(), "secret"))
	var record map[string]any
	is.NoErr(json.NewDecoder(&logs).Decode(&record))
	is.Equal(record["level"], "ERROR")
	is.Equal(record["status"], float64(500))
	is.Equal(record["error"], "postgres: secret password")
	is.Equal(record["method"], "GET")
	is.Equal(record["path"], "/boards")
	k.Debug = true
	is.True(strings.Contains(serve(t, route, "application/json").Body.String(), "postgres: secret password"))
	route = k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(404).Error(errors.New("missing board"))
	})
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 404)
}

func TestErrorf(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	route := k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(503).Errorf("load board %d: %w", 42, errors.New("database unavailable"))
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 503)
	is.Equal(w.Body.String(), "{\"error\":\"Service Unavailable\"}\n")
	var record map[string]any
	is.NoErr(json.NewDecoder(&logs).Decode(&record))
	is.Equal(record["error"], "load board 42: database unavailable")
	k.Debug = true
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 503)
	is.True(strings.Contains(w.Body.String(), "load board 42: database unavailable"))
}

func TestMessageFormattingAndStatus(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	underlying := errors.New("database: secret")
	route := k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(http.StatusConflict).Error(underlying).Message("board %d already exists", 42)
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, http.StatusConflict)
	is.Equal(w.Body.String(), "{\"error\":\"board 42 already exists\"}\n")
	k.Debug = true
	w = serve(t, route, "application/json")
	is.Equal(w.Body.String(), "{\n\t\"error\": \"board 42 already exists\"\n}\n")
	w = serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusConflict)
	is.True(strings.Contains(w.Body.String(), "board 42 already exists"))
}

func TestErrorView(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	view := writeView(t, `<h1>{{.Status}} {{.Message}}</h1><p>{{index .Fields "name"}}</p>`)
	route := k.Show(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(http.StatusNotFound).Error(errors.New("missing board")).Message("Board not found")
	}).ErrorView(view)
	w := serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusNotFound)
	is.Equal(w.Body.String(), "<h1>404 Board not found</h1><p>[]</p>")

	validationView := writeView(t, `<h1>{{.Message}}</h1><p>{{index .Fields "name"}}</p>`)
	type input struct {
		Name string `json:"name" validate:"required"`
	}
	validationRoute := k.New(func(ctx context.Context, req *koi.Request[input]) (res koi.Response[board]) {
		return res.Body(board{ID: "42"})
	}).ErrorView(validationView)
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	validationRoute.ServeHTTP(w, req)
	is.Equal(w.Code, http.StatusUnprocessableEntity)
	is.Equal(w.Body.String(), "<h1>Name is required</h1><p>[is required]</p>")
}

func TestErrorViewFallback(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Show(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(http.StatusNotFound).Error(errors.New("missing board"))
	}).ErrorView(filepath.Join(t.TempDir(), "missing.html"))
	w := serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusNotFound)
	is.True(strings.Contains(w.Body.String(), "Not Found"))
}

func TestErrorViewPartialFallback(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Show(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(404).Header("Content-Length", "999").Error(errors.New("private"))
	}).ErrorView(writeView(t, `partial secret {{.Missing}}`))
	w := serve(t, route, "text/html")
	is.Equal(w.Code, 404)
	is.Equal(w.Header().Get("Content-Length"), "")
	is.True(!strings.Contains(w.Body.String(), "partial secret"))
	is.True(strings.Contains(w.Body.String(), "Not Found"))
}

func TestErrorViewEmpty(t *testing.T) {
	is := is.New(t)
	route := koi.New().Show(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(404).Error(errors.New("private"))
	}).ErrorView(writeView(t, ""))
	w := serve(t, route, "text/html")
	is.Equal(w.Code, 404)
	is.Equal(w.Body.Len(), 0)
}

func TestRenderFailureBuffered(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	view := writeView(t, `partial secret {{.Missing}}`)
	w := serve(t, k.Show(boardHandler).View(view), "text/html")
	is.Equal(w.Code, 500)
	is.True(!strings.Contains(w.Body.String(), "partial secret"))
	is.True(strings.Contains(w.Body.String(), "Internal Server Error"))
	route := k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[any]) {
		return res.Body(make(chan int))
	})
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
}

func TestNegotiation(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	view := writeView(t, `html`)
	route := k.Wrap(boardHandler).View(view)
	w := serve(t, route, "")
	is.Equal(w.Header().Get("Content-Type"), "application/json")
	w = serve(t, route, "text/html;q=0, application/json")
	is.Equal(w.Header().Get("Content-Type"), "application/json")
	w = serve(t, route, "text/html;q=1, application/json;q=0.5")
	is.Equal(w.Body.String(), "html")
	w = serve(t, route, "application/json;q=0, text/html;q=0")
	is.Equal(w.Code, 406)
	w = serve(t, route, "image/png")
	is.Equal(w.Code, 406)
	req := &koi.Request[koi.Empty]{Request: httptest.NewRequest("GET", "/", nil)}
	req.Header.Set("Accept", "*/*;q=1, application/json;q=0")
	is.True(!req.Accepts("json"))
	is.True(req.Accepts("html"))
}

func TestValidationJSON(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	type input struct {
		Name string `json:"name" validate:"required"`
	}
	called := false
	route := k.Create(func(ctx context.Context, req *koi.Request[input]) (res koi.Response[board]) {
		called = true
		return res.Body(board{ID: "42"})
	})
	req := httptest.NewRequest("POST", "/boards", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 422)
	is.True(!called)
	var body struct {
		Fields map[string][]string `json:"fields"`
	}
	is.NoErr(json.Unmarshal(w.Body.Bytes(), &body))
	is.Equal(body.Fields["name"], []string{"is required"})
}

func TestAcceptsMediaTypes(t *testing.T) {
	is := is.New(t)
	req := &koi.Request[koi.Empty]{Request: httptest.NewRequest("GET", "/", nil)}
	req.Header.Set("Accept", "image/png, application/json;q=0")
	is.True(req.Accepts("image/png"))
	is.True(!req.Accepts("image/jpeg"))
	is.True(!req.Accepts("json"))
	is.True(!req.Accepts("html"))
}

func TestVaryPreserved(t *testing.T) {
	is := is.New(t)
	route := koi.New().Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Header("Vary", "Origin").Body(board{ID: "42"})
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Header().Values("Vary"), []string{"Accept", "Origin"})
}

type pointerInput struct {
	Name string `json:"name"`
}

func (v *pointerInput) Validate() koi.Invalid {
	return koi.Invalid{"name": {"is required"}}
}

func TestPointerInputValidation(t *testing.T) {
	is := is.New(t)
	called := false
	k := koi.New()
	route := k.Wrap(func(ctx context.Context, req *koi.Request[*pointerInput]) (res koi.Response[board]) {
		called = true
		return res.Body(board{ID: "42"})
	})
	req := httptest.NewRequest("POST", "/boards", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 422)
	is.True(!called)
	is.Equal(w.Body.String(), "{\"error\":\"Name is required\",\"fields\":{\"name\":[\"is required\"]}}\n")
	k.Debug = true
	req = httptest.NewRequest("POST", "/boards", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Body.String(), "{\n\t\"error\": \"Name is required\",\n\t\"fields\": {\n\t\t\"name\": [\n\t\t\t\"is required\"\n\t\t]\n\t}\n}\n")
}

func TestHead(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	req := httptest.NewRequest(http.MethodHead, "/boards", nil)
	w := httptest.NewRecorder()
	k.Show(boardHandler).ServeHTTP(w, req)
	is.Equal(w.Code, 200)
	is.Equal(w.Body.Len(), 0)
	is.Equal(w.Header().Get("Content-Type"), "application/json")
	w = httptest.NewRecorder()
	k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Error(errors.New("private"))
	}).ServeHTTP(w, req)
	is.Equal(w.Code, 500)
	is.Equal(w.Body.Len(), 0)
}

func TestResponseCopies(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	base := koi.Response[board]{}.Header("X-Value", "base")
	changed := base.Header("X-Value", "changed")
	route := k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) koi.Response[board] {
		return base.Body(board{ID: "42"})
	})
	w := serve(t, route, "")
	is.Equal(w.Header().Get("X-Value"), "base")
	route = k.Wrap(func(ctx context.Context, req *koi.Request[koi.Empty]) koi.Response[board] { return changed })
	w = serve(t, route, "")
	is.Equal(w.Header().Get("X-Value"), "changed")
}

func writeView(t testing.TB, content string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "view.html")
	err := os.WriteFile(name, []byte(content), 0600)
	is.New(t).NoErr(err)
	return name
}
