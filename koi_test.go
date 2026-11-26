package koi_test

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/koi"
)

type board struct {
	ID string `json:"id"`
}

type readTracker struct {
	read bool
}

func (r *readTracker) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func boardHandler(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
	return res.Body(board{ID: "42"})
}

func TestEmptyDoesNotReadBody(t *testing.T) {
	is := is.New(t)
	body := &readTracker{}
	req := httptest.NewRequest(http.MethodPost, "/boards", body)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	koi.New().Wrap(boardHandler).ServeHTTP(w, req)

	is.Equal(w.Code, http.StatusOK)
	is.True(!body.read)
}

func serve(t testing.TB, h http.Handler, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/boards", nil)
	req.Header.Set("Accept", accept)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestWrapStatuses(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	w := serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler).Status(http.StatusCreated), "application/json")
	is.Equal(w.Code, 201)
	w = serve(t, k.Wrap(boardHandler), "application/json")
	is.Equal(w.Code, 200)
	w = serve(t, k.Wrap(boardHandler).Status(http.StatusNoContent), "application/json")
	is.Equal(w.Code, 204)
}

func TestStatusOverrides(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(boardHandler).Status(http.StatusAccepted).Redirect("42")
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 202)
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
	w = serve(t, route, "text/html")
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/boards/42")
	route = k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(boardHandler).View(view)
	w := serve(t, route, "text/html")
	is.Equal(w.Body.String(), "<h1>42</h1>")
	is.Equal(w.Header().Get("Content-Type"), "text/html; charset=utf-8")
	w = serve(t, route, "application/json")
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
	is.Equal(w.Header().Get("Vary"), "Accept")
	route = k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Redirect("/login")
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 303)
	w = serve(t, route, "text/html")
	is.Equal(w.Header().Get("Location"), "/login")
	route = k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Redirect("chosen").Body(board{ID: "42"})
	}).Redirect("default")
	w = serve(t, route, "text/html")
	is.Equal(w.Header().Get("Location"), "/boards/chosen")
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 200)
	emptyRoute := k.Wrap(emptyHandler).Redirect("10")
	w = serve(t, emptyRoute, "text/html")
	is.Equal(w.Header().Get("Location"), "/boards/10")
	responseRoute := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[koi.Empty]) {
		return res.Redirect("10")
	})
	w = serve(t, responseRoute, "application/json")
	is.Equal(w.Header().Get("Location"), "/boards/10")
	sameRoute := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Body(board{ID: "42"}).Redirect(".")
	})
	sameReq := httptest.NewRequest("PATCH", "/agents/random-agent", nil)
	sameReq.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	sameRoute.ServeHTTP(w, sameReq)
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/agents/random-agent")
	parentRoute := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[koi.Empty]) {
		return res.Redirect("..")
	})
	parentReq := httptest.NewRequest("PATCH", "/agents/random-agent", nil)
	parentReq.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	parentRoute.ServeHTTP(w, parentReq)
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/agents")
	nestedRoute := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[koi.Empty]) {
		return res.Redirect("../some/path")
	})
	nestedReq := httptest.NewRequest("PATCH", "/agents/random-agent", nil)
	nestedReq.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	nestedRoute.ServeHTTP(w, nestedReq)
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/agents/some/path")
	req := httptest.NewRequest("GET", "/boards?page=2", nil)
	req.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	responseRoute.ServeHTTP(w, req)
	is.Equal(w.Header().Get("Location"), "/boards/10?page=2")
	deletion := k.Wrap(emptyHandler).Status(http.StatusNoContent).Redirect("/boards")
	w = serve(t, deletion, "text/html")
	is.Equal(w.Code, 303)
	w = serve(t, deletion, "application/json")
	is.Equal(w.Code, 204)
	is.Equal(w.Body.Len(), 0)
	w = serve(t, k.Wrap(emptyHandler), "application/json")
	is.Equal(w.Code, 204)
}

func TestErrorPrivacyAndLogging(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Error(errors.New("postgres: secret password"))
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
	is.True(!strings.Contains(serve(t, route, "text/html").Body.String(), "secret"))
	var record map[string]any
	is.NoErr(json.UnmarshalDecode(jsontext.NewDecoder(&logs), &record))
	is.Equal(record["level"], "ERROR")
	is.Equal(record["status"], float64(500))
	is.Equal(record["error"], "postgres: secret password")
	is.Equal(record["method"], "GET")
	is.Equal(record["path"], "/boards")
	k.Debug = true
	is.True(strings.Contains(serve(t, route, "application/json").Body.String(), "postgres: secret password"))
	route = k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(503).Errorf("load board %d: %w", 42, errors.New("database unavailable"))
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 503)
	is.Equal(w.Body.String(), "{\"error\":\"Service Unavailable\"}\n")
	var record map[string]any
	is.NoErr(json.UnmarshalDecode(jsontext.NewDecoder(&logs), &record))
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
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(http.StatusNotFound).Error(errors.New("missing board")).Message("Board not found")
	}).ErrorView(view)
	w := serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusNotFound)
	is.Equal(w.Body.String(), "<h1>404 Board not found</h1><p>[]</p>")

	validationView := writeView(t, `<h1>{{.Message}}</h1><p>{{index .Fields "name"}}</p>`)
	type input struct {
		Name string `json:"name" validate:"required"`
	}
	validationRoute := k.Wrap(func(req *koi.Request[input]) (res koi.Response[board]) {
		return res.Body(board{ID: "42"})
	}).ErrorView(validationView)
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	validationRoute.ServeHTTP(w, req)
	is.Equal(w.Code, http.StatusUnprocessableEntity)
	is.Equal(w.Body.String(), "<h1>name is required</h1><p>[is required]</p>")
}

func TestErrorViewFallback(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(http.StatusNotFound).Error(errors.New("missing board"))
	}).ErrorView(filepath.Join(t.TempDir(), "missing.html"))
	w := serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusNotFound)
	is.True(strings.Contains(w.Body.String(), "Koi"))
	is.True(strings.Contains(w.Body.String(), "Not Found"))
	is.True(strings.Contains(w.Body.String(), "<pre>"))
}

func TestErrorViewPartialFallback(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(404).Header("Content-Length", "999").Header("X-Board", "42").Error(errors.New("private"))
	}).ErrorView(writeView(t, `partial secret {{.Missing}}`))
	w := serve(t, route, "text/html")
	is.Equal(w.Code, 404)
	is.Equal(w.Header().Get("Content-Length"), "")
	is.Equal(w.Header().Get("X-Board"), "42")
	is.True(!strings.Contains(w.Body.String(), "partial secret"))
	is.True(strings.Contains(w.Body.String(), "Not Found"))
}

func TestErrorViewEmpty(t *testing.T) {
	is := is.New(t)
	route := koi.New().Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	w := serve(t, k.Wrap(boardHandler).View(view), "text/html")
	is.Equal(w.Code, 500)
	is.True(!strings.Contains(w.Body.String(), "partial secret"))
	is.True(strings.Contains(w.Body.String(), "Internal Server Error"))
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[any]) {
		return res.Body(make(chan int))
	})
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
}

func TestMissingHTMLView(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Debug = true
	route := k.Wrap(boardHandler)
	w := serve(t, route, "text/html")
	is.Equal(w.Code, http.StatusInternalServerError)
	is.True(strings.Contains(w.Body.String(), "HTML response has no view"))
	parent := k.Wrap(navHandler)
	w = serve(t, k.Wrap(boardHandler).View("page").Parent(parent), "text/html")
	is.Equal(w.Code, http.StatusInternalServerError)
	is.True(strings.Contains(w.Body.String(), "parent has no view"))
}

func TestMissingHTMLViewJSON(t *testing.T) {
	is := is.New(t)
	route := koi.New().Wrap(boardHandler)
	w := serve(t, route, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	is.Equal(w.Code, http.StatusOK)
	is.Equal(w.Header().Get("Content-Type"), "application/json")
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
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
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	type input struct {
		Name string `json:"name" validate:"required"`
	}
	called := false
	route := k.Wrap(func(req *koi.Request[input]) (res koi.Response[board]) {
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
	is.True(strings.Contains(logs.String(), `"status":422`))
	var body struct {
		Fields map[string][]string `json:"fields"`
	}
	is.NoErr(json.Unmarshal(w.Body.Bytes(), &body))
	is.Equal(body.Fields["name"], []string{"is required"})
	// Field errors from a controller are logged with the status that was sent.
	logs.Reset()
	route = k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Error(koi.Invalid{"name": {"is taken"}})
	})
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 422)
	is.True(strings.Contains(logs.String(), `"status":422`))
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
	route := koi.New().Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(func(req *koi.Request[*pointerInput]) (res koi.Response[board]) {
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
	is.Equal(w.Body.String(), "{\"error\":\"name is required\",\"fields\":{\"name\":[\"is required\"]}}\n")
	k.Debug = true
	req = httptest.NewRequest("POST", "/boards", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Body.String(), "{\n\t\"error\": \"name is required\",\n\t\"fields\": {\n\t\t\"name\": [\n\t\t\t\"is required\"\n\t\t]\n\t}\n}\n")
}

func TestHead(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	req := httptest.NewRequest(http.MethodHead, "/boards", nil)
	w := httptest.NewRecorder()
	k.Wrap(boardHandler).ServeHTTP(w, req)
	is.Equal(w.Code, 200)
	is.Equal(w.Body.Len(), 0)
	is.Equal(w.Header().Get("Content-Type"), "application/json")
	w = httptest.NewRecorder()
	k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
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
	route := k.Wrap(func(req *koi.Request[koi.Empty]) koi.Response[board] {
		return base.Body(board{ID: "42"})
	})
	w := serve(t, route, "")
	is.Equal(w.Header().Get("X-Value"), "base")
	route = k.Wrap(func(req *koi.Request[koi.Empty]) koi.Response[board] { return changed })
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

type nameInput struct {
	Name string `json:"name" form:"name"`
}

func nameHandler(req *koi.Request[nameInput]) (res koi.Response[koi.Empty]) {
	return res
}

func post(t testing.TB, h http.Handler, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestMaxBodySize(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	is.Equal(k.MaxBodySize, int64(10<<20))
	k.MaxBodySize = 16
	route := k.Wrap(nameHandler)

	w := post(t, route, "application/json", `{"name":"matt"}`)
	is.Equal(w.Code, http.StatusNoContent)

	w = post(t, route, "application/json", `{"name":"matthew mueller"}`)
	is.Equal(w.Code, http.StatusRequestEntityTooLarge)
	is.Equal(w.Body.String(), "{\"error\":\"Request Entity Too Large\"}\n")

	w = post(t, route, "application/x-www-form-urlencoded", "name=matthew+mueller")
	is.Equal(w.Code, http.StatusRequestEntityTooLarge)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	is.NoErr(mw.WriteField("name", "matthew mueller"))
	is.NoErr(mw.Close())
	w = post(t, route, mw.FormDataContentType(), body.String())
	is.Equal(w.Code, http.StatusRequestEntityTooLarge)

	k.MaxBodySize = 0
	w = post(t, k.Wrap(nameHandler), "application/json", `{"name":"matthew mueller"}`)
	is.Equal(w.Code, http.StatusNoContent)
}

func TestUnsupportedMediaTypeStatus(t *testing.T) {
	is := is.New(t)
	w := post(t, koi.New().Wrap(nameHandler), "text/plain", "matt")
	is.Equal(w.Code, http.StatusUnsupportedMediaType)
	is.Equal(w.Body.String(), "{\"error\":\"unsupported media type \\\"text/plain\\\"\"}\n")
}

type validatedBoard struct {
	ID string `json:"id" validate:"required"`
}

func (b *validatedBoard) Validate() koi.Invalid {
	if b.ID == "secret" {
		return koi.Invalid{"id": {"is reserved"}}
	}
	return nil
}

func TestResponseValidation(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	id := ""
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[validatedBoard]) {
		return res.Body(validatedBoard{ID: id})
	}).View("missing.html")
	// Invalid bodies are server errors, and their fields stay private.
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
	var record map[string]any
	is.NoErr(json.UnmarshalDecode(jsontext.NewDecoder(&logs), &record))
	is.Equal(record["error"], "koi: invalid response body: id is required")
	// HTML responses are validated before rendering.
	w = serve(t, route, "text/html")
	is.Equal(w.Code, 500)
	is.True(!strings.Contains(w.Body.String(), "is required"))
	// Custom Validate methods also run.
	id = "secret"
	k.Debug = true
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 500)
	is.True(strings.Contains(w.Body.String(), "koi: invalid response body: id is reserved"))
	// Valid bodies are sent.
	id = "42"
	w = serve(t, route, "application/json")
	is.Equal(w.Code, 200)
}

type boardLanes struct {
	Lanes []string       `json:"lanes"`
	Meta  map[string]int `json:"meta"`
}

func TestNilSlicesEncodeEmpty(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[boardLanes]) {
		return res.Body(boardLanes{})
	})
	w := serve(t, route, "application/json")
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "{\"lanes\":[],\"meta\":{}}\n")
}

type eventQuery struct {
	Types []string `form:"types" json:"types"`
	Limit int      `form:"limit" json:"limit"`
	Order string   `form:"order" json:"order"`
}

func TestQueryArrays(t *testing.T) {
	is := is.New(t)
	route := koi.New().Wrap(func(req *koi.Request[eventQuery]) (res koi.Response[eventQuery]) {
		return res.Body(req.Input)
	})
	// Encoded like anthropic-sdk-go's ArrayQueryFormatBrackets
	req := httptest.NewRequest(http.MethodGet, "/events?limit=1&order=desc&types%5B%5D=session.status_idle&types%5B%5D=session.error", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, http.StatusOK)
	is.Equal(w.Body.String(), `{"types":["session.status_idle","session.error"],"limit":1,"order":"desc"}`+"\n")
}

// stackRenderer writes the name and props of each view it's given, in order.
type stackRenderer struct {
	calls int
}

func (r *stackRenderer) Render(ctx context.Context, w io.Writer, views ...koi.View) error {
	r.calls++
	for _, view := range views {
		fmt.Fprintf(w, "%s:%v\n", view.Name, view.Props)
	}
	return nil
}

type navInput struct {
	Tab string `form:"tab"`
}

type nav struct {
	Tab string
}

func navHandler(req *koi.Request[navInput]) (res koi.Response[nav]) {
	return res.Body(nav{Tab: req.Input.Tab})
}

func emptyHandler(req *koi.Request[koi.Empty]) (res koi.Response[koi.Empty]) {
	return res
}

func getHtml(t testing.TB, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestParentTemplate(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	// Each parent's slot holds everything inside it, at any depth.
	root := k.Wrap(emptyHandler).View(writeView(t, `<html>{{slot}}</html>`))
	shell := k.Wrap(navHandler).View(writeView(t, `<nav>{{.Tab}}</nav><main>{{slot}}</main>`)).Parent(root)
	section := k.Wrap(emptyHandler).View(writeView(t, `<section>{{slot}}</section>`)).Parent(shell)
	page := k.Wrap(boardHandler).View(writeView(t, `<h1>{{.ID}}</h1>`)).Parent(section)
	w := getHtml(t, page, "/boards?tab=usage")
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "<html><nav>usage</nav><main><section><h1>42</h1></section></main></html>")
}

func TestParentStack(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Renderer = &stackRenderer{}
	layout := k.Wrap(emptyHandler).View("layout")
	shell := k.Wrap(navHandler).View("shell").Parent(layout)
	page := k.Wrap(boardHandler).View("page").Parent(shell)
	w := getHtml(t, page, "/boards?tab=billing")
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "layout:{}\nshell:{billing}\npage:{42}\n")
	// Pages share a stack by sharing the parent.
	other := k.Wrap(boardHandler).View("other").Parent(shell)
	w = getHtml(t, other, "/boards?tab=usage")
	is.Equal(w.Body.String(), "layout:{}\nshell:{usage}\nother:{42}\n")
	// Response.Render picks the view, for a page and for a parent.
	compact := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Render("compact", nav{Tab: "billing"})
	}).View("shell")
	edit := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Render("edit", board{ID: "42"})
	}).View("page").Parent(compact)
	w = getHtml(t, edit, "/boards")
	is.Equal(w.Body.String(), "compact:{billing}\nedit:{42}\n")
}

func TestParentJSON(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	called := false
	shell := k.Wrap(func(req *koi.Request[navInput]) (res koi.Response[nav]) {
		called = true
		return navHandler(req)
	}).View("shell")
	page := k.Wrap(boardHandler).View("page").Parent(shell)
	w := serve(t, page, "application/json")
	is.Equal(w.Body.String(), "{\"id\":\"42\"}\n")
	is.True(!called)
}

func TestParentPanic(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	renderer := &stackRenderer{}
	k.Renderer = renderer
	shell := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		panic("boom")
	}).View("shell")
	w := getHtml(t, k.Wrap(boardHandler).View("page").Parent(shell), "/boards")
	is.Equal(w.Code, 500)
	is.Equal(renderer.calls, 0)
	is.True(!strings.Contains(w.Body.String(), "boom"))
	is.True(strings.Contains(logs.String(), "koi: parent panicked: boom"))
}

func TestParentConcurrent(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Renderer = &stackRenderer{}
	pageStarted, shellStarted := make(chan struct{}), make(chan struct{})
	// Each handler waits for the other to start, which only works if they overlap.
	shell := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		close(shellStarted)
		select {
		case <-pageStarted:
			return res.Body(nav{Tab: "shell"})
		case <-time.After(5 * time.Second):
			return res.Errorf("page never started")
		}
	}).View("shell")
	page := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		close(pageStarted)
		select {
		case <-shellStarted:
			return res.Body(board{ID: "42"})
		case <-time.After(5 * time.Second):
			return res.Errorf("shell never started")
		}
	}).View("page").Parent(shell)
	w := getHtml(t, page, "/boards")
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "shell:{shell}\npage:{42}\n")
}

func TestParentBody(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Renderer = &stackRenderer{}
	// Both bind the same input, but only the page reads the body.
	shell := k.Wrap(func(req *koi.Request[nameInput]) (res koi.Response[nav]) {
		return res.Body(nav{Tab: req.Input.Name})
	}).View("shell")
	page := k.Wrap(func(req *koi.Request[nameInput]) (res koi.Response[board]) {
		return res.Body(board{ID: req.Input.Name})
	}).View("page").Parent(shell)
	req := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(`{"name":"matt"}`))
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	page.ServeHTTP(w, req)
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "shell:{}\npage:{matt}\n")
}

func TestParentCanceled(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	renderer := &stackRenderer{}
	k.Renderer = renderer
	canceled := false
	shell := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		select {
		case <-req.Context().Done():
			canceled = true
		case <-time.After(5 * time.Second):
		}
		return res
	}).View("shell")
	redirect := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Redirect("/login")
	}).View("page").Parent(shell)
	w := getHtml(t, redirect, "/boards")
	is.Equal(w.Code, 303)
	is.True(canceled)
	canceled = false
	failure := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Status(404).Errorf("missing board")
	}).View("page").Parent(shell)
	w = getHtml(t, failure, "/boards")
	is.Equal(w.Code, 404)
	is.True(canceled)
	is.Equal(renderer.calls, 0)
}

func TestParentFailures(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	renderer := &stackRenderer{}
	k.Renderer = renderer
	signedOut := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Status(401).Errorf("signed out")
	}).View("layout")
	w := getHtml(t, k.Wrap(boardHandler).View("page").Parent(signedOut), "/boards")
	is.Equal(w.Code, 401)
	is.True(strings.Contains(w.Body.String(), "Unauthorized"))
	login := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Redirect("/login")
	}).View("shell")
	w = getHtml(t, k.Wrap(boardHandler).View("page").Parent(login), "/boards")
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("Location"), "/login")
	// A parent's outcome also replaces a page that has no body to render.
	w = getHtml(t, k.Wrap(emptyHandler).View("page").Parent(login), "/boards")
	is.Equal(w.Code, 303)
	// Input errors are reported like the page's.
	events := k.Wrap(func(req *koi.Request[eventQuery]) (res koi.Response[eventQuery]) {
		return res.Body(req.Input)
	}).View("shell")
	w = getHtml(t, k.Wrap(boardHandler).View("page").Parent(events), "/boards?limit=many")
	is.Equal(w.Code, 400)
	// When several parents fail, the outermost one wins.
	forbidden := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Status(403).Errorf("forbidden")
	}).View("shell").Parent(signedOut)
	w = getHtml(t, k.Wrap(boardHandler).View("page").Parent(forbidden), "/boards")
	is.Equal(w.Code, 401)
	is.Equal(renderer.calls, 0)
}

func TestParentHeaders(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Renderer = &stackRenderer{}
	layout := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Header("X-Layout", "1").Header("Cache-Control", "public").Header("X-Frame", "layout")
	}).View("layout")
	shell := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Header("Vary", "Cookie").Header("Cache-Control", "private").Header("X-Frame", "shell")
	}).View("shell").Parent(layout)
	noStore := func(req *koi.Request[koi.Empty]) (res koi.Response[board]) {
		return res.Header("Cache-Control", "no-store").Body(board{ID: "42"})
	}
	w := getHtml(t, k.Wrap(noStore).View("page").Parent(shell), "/boards")
	is.Equal(w.Code, 200)
	is.Equal(w.Header().Get("X-Layout"), "1")
	is.Equal(w.Header().Values("Vary"), []string{"Accept", "Cookie"})
	// The page wins over its parents, and an inner parent over an outer one.
	is.Equal(w.Header().Get("Cache-Control"), "no-store")
	is.Equal(w.Header().Get("X-Frame"), "shell")
	// A redirect or error carries the headers of the route that returned it,
	// and not those of the page or of the other parents.
	login := k.Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[nav]) {
		return res.Header("X-Frame", "login").Redirect("/login")
	}).View("shell").Parent(layout)
	w = getHtml(t, k.Wrap(noStore).View("page").Parent(login), "/boards")
	is.Equal(w.Code, 303)
	is.Equal(w.Header().Get("X-Frame"), "login")
	is.Equal(w.Header().Get("Cache-Control"), "")
	is.Equal(w.Header().Get("X-Layout"), "")
}

type sandboxInput struct {
	ID         string `json:"id"`
	Limit      int    `json:"limit"`
	Networking struct {
		Type         string   `json:"type"`
		AllowedHosts []string `json:"allowed_hosts"`
	} `json:"networking"`
}

func sandboxHandler(req *koi.Request[sandboxInput]) (res koi.Response[sandboxInput]) {
	return res.Body(req.Input)
}

func TestInputErrors(t *testing.T) {
	is := is.New(t)
	route := koi.New().Wrap(sandboxHandler)
	// A value of the wrong type is reported like a validation failure, which
	// says what was sent.
	w := post(t, route, "application/json", `{"networking":{"type":5}}`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"networking.type can't be a number","fields":{"networking.type":["can't be a number"]}}`+"\n")
	w = post(t, route, "application/json", `{"networking":{"allowed_hosts":["a",{}]}}`)
	is.Equal(w.Body.String(), `{"error":"networking.allowed_hosts.1 can't be an object","fields":{"networking.allowed_hosts.1":["can't be an object"]}}`+"\n")
	w = post(t, route, "application/json", `{"limit":"many","networking":[]}`)
	is.Equal(w.Body.String(), `{"error":"limit can't be a string","fields":{"limit":["can't be a string"]}}`+"\n")
	w = post(t, route, "application/json", `{"networking":true}`)
	is.Equal(w.Body.String(), `{"error":"networking can't be a boolean","fields":{"networking":["can't be a boolean"]}}`+"\n")
	// The value at the root isn't a field.
	w = post(t, route, "application/json", `[1]`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"body can't be a list"}`+"\n")
	// The JSON type fits, but the value doesn't.
	w = post(t, route, "application/json", `{"limit":1.5}`)
	is.Equal(w.Body.String(), `{"error":"limit is invalid","fields":{"limit":["is invalid"]}}`+"\n")
	// Other errors that only describe the request keep their status and show
	// their own text.
	w = post(t, route, "application/json", `{"id":"a",}`)
	is.Equal(w.Code, 400)
	is.Equal(w.Body.String(), `{"error":"jsontext: invalid character ',' at start of value after offset 9"}`+"\n")
	w = post(t, route, "application/json", `{"id":"a","id":"b"}`)
	is.Equal(w.Body.String(), `{"error":"jsontext: duplicate object member name \"id\""}`+"\n")
	w = post(t, route, "application/x-www-form-urlencoded", `id=%zz`)
	is.Equal(w.Body.String(), `{"error":"invalid URL escape \"%zz\""}`+"\n")
	w = post(t, route, "application/x-www-form-urlencoded", `id[=a`)
	is.Equal(w.Body.String(), `{"error":"invalid request key \"id[\""}`+"\n")
}

// replica fails to decode with an error that describes the server.
type replica string

func (r *replica) UnmarshalText(text []byte) error {
	return errors.New("dial tcp 10.2.0.7:5432: connection refused")
}

func TestInputErrorsPrivate(t *testing.T) {
	is := is.New(t)
	type input struct {
		Replica replica  `json:"replica"`
		Events  chan int `json:"events"`
	}
	k := koi.New()
	var logs bytes.Buffer
	k.Log = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	route := k.Wrap(func(req *koi.Request[input]) (res koi.Response[koi.Empty]) {
		return res
	})
	// A body that fails to read says where the server was reading from.
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", iotest.ErrReader(errors.New("read tcp 10.0.0.5:8080->10.0.0.9:51234: i/o timeout")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 400)
	is.Equal(w.Body.String(), "{\"error\":\"Bad Request\"}\n")
	is.True(strings.Contains(logs.String(), "10.0.0.5:8080"))
	// A type that decodes itself can fail with anything.
	w = post(t, route, "application/x-www-form-urlencoded", `replica=west`)
	is.Equal(w.Code, 400)
	is.Equal(w.Body.String(), "{\"error\":\"Bad Request\"}\n")
	logs.Reset()
	w = post(t, route, "application/json", `{"replica":"west"}`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"replica is invalid","fields":{"replica":["is invalid"]}}`+"\n")
	is.True(strings.Contains(logs.String(), "dial tcp 10.2.0.7:5432"))
	// A field that can't be decoded at all is the server's bug, not the client's.
	w = post(t, route, "application/json", `{"events":1}`)
	is.Equal(w.Code, 500)
	is.Equal(w.Body.String(), "{\"error\":\"Internal Server Error\"}\n")
	is.True(strings.Contains(logs.String(), "chan int"))
	// A form value of the wrong type prints the Go type it was meant for.
	w = post(t, koi.New().Wrap(sandboxHandler), "application/x-www-form-urlencoded", `networking=5`)
	is.Equal(w.Code, 400)
	is.Equal(w.Body.String(), "{\"error\":\"Bad Request\"}\n")
}

type mode string

func (m *mode) UnmarshalText(text []byte) error {
	return koi.Invalid{"mode": {"has to be limited"}}
}

func TestInputErrorsCustom(t *testing.T) {
	is := is.New(t)
	type input struct {
		Mode mode `json:"mode"`
	}
	route := koi.New().Wrap(func(req *koi.Request[input]) (res koi.Response[koi.Empty]) {
		return res
	})
	// A type that decodes itself can return its own field errors.
	w := post(t, route, "application/json", `{"mode":"open"}`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"mode has to be limited","fields":{"mode":["has to be limited"]}}`+"\n")
}

func TestUnknownFields(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	route := k.Wrap(sandboxHandler)
	w := post(t, route, "application/json", `{"netwrking":{}}`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"netwrking isn't a known field","fields":{"netwrking":["isn't a known field"]}}`+"\n")
	// Nested fields are checked too, and named by their path.
	w = post(t, route, "application/json", `{"networking":{"allowed_host":[]}}`)
	is.Equal(w.Body.String(), `{"error":"networking.allowed_host isn't a known field","fields":{"networking.allowed_host":["isn't a known field"]}}`+"\n")
	// Forms and query strings carry values meant for others, so they stay lenient.
	w = post(t, route, "application/x-www-form-urlencoded", `csrf=token&limit=2`)
	is.Equal(w.Code, 200)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes?utm_source=mail", strings.NewReader(`{"limit":2}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	route.ServeHTTP(w, req)
	is.Equal(w.Code, 200)
	// They can be allowed, which ignores them.
	k.AllowUnknownFields = true
	w = post(t, route, "application/json", `{"netwrking":{}}`)
	is.Equal(w.Code, 200)
}

func TestPathValues(t *testing.T) {
	is := is.New(t)
	k := koi.New()
	k.Renderer = &stackRenderer{}
	show := func(req *koi.Request[sandboxInput]) (res koi.Response[board]) {
		return res.Body(board{ID: req.Input.ID})
	}
	shell := k.Wrap(func(req *koi.Request[sandboxInput]) (res koi.Response[nav]) {
		return res.Body(nav{Tab: req.Input.ID})
	}).View("shell")
	mux := http.NewServeMux()
	mux.Handle("/sandboxes/{id}", k.Wrap(show).View("page").Parent(shell))
	mux.Handle("GET /files/{id...}", k.Wrap(show))
	request := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	// The path wins over a query value of the same name, however it's spelled.
	// Without that, which of id and ID wins changes from request to request.
	for range 20 {
		w := request("DELETE", "/sandboxes/abc?id=other&ID=other&id[]=other", "")
		is.Equal(w.Body.String(), "{\"id\":\"abc\"}\n")
	}
	w := request("PUT", "/sandboxes/abc", "null")
	is.Equal(w.Body.String(), "{\"id\":\"abc\"}\n")
	w = request("GET", "/files/a/b.txt", "")
	is.Equal(w.Body.String(), "{\"id\":\"a/b.txt\"}\n")
	// A body field of the same name conflicts, as it does with the query.
	w = request("PUT", "/sandboxes/abc", `{"id":"other"}`)
	is.Equal(w.Code, 400)
	is.Equal(w.Body.String(), "{\"error\":\"duplicate request field \\\"id\\\"\"}\n")
	// JSON matches names loosely, so the conflict has to as well.
	w = request("PUT", "/sandboxes/abc", `{"_I-D":"other"}`)
	is.Equal(w.Code, 400)
	// Other routers set the pattern too, with their own wildcard syntax.
	req := httptest.NewRequest("GET", "/boards/12", nil)
	req.Pattern = "/boards/{id:[0-9]+}"
	req.SetPathValue("id", "12")
	w = httptest.NewRecorder()
	k.Wrap(show).ServeHTTP(w, req)
	is.Equal(w.Body.String(), "{\"id\":\"12\"}\n")
	// Parents bind path values too.
	w = getHtml(t, mux, "/sandboxes/abc")
	is.Equal(w.Body.String(), "shell:{abc}\npage:{abc}\n")
}

// pause is a duration that chooses its own format, since JSON has none for a
// time.Duration.
type pause time.Duration

func (p pause) MarshalText() ([]byte, error) {
	return []byte(time.Duration(p).String()), nil
}

func (p *pause) UnmarshalText(text []byte) error {
	d, err := time.ParseDuration(string(text))
	*p = pause(d)
	return err
}

func TestDurations(t *testing.T) {
	is := is.New(t)
	type input struct {
		Wait pause `json:"wait"`
	}
	route := koi.New().Wrap(func(req *koi.Request[input]) (res koi.Response[input]) {
		return res.Body(req.Input)
	})
	// A type that marshals itself is decoded and encoded its own way, in JSON
	// and in forms.
	w := post(t, route, "application/json", `{"wait":"1.5s"}`)
	is.Equal(w.Code, 200)
	is.Equal(w.Body.String(), "{\"wait\":\"1.5s\"}\n")
	w = post(t, route, "application/x-www-form-urlencoded", `wait=90s`)
	is.Equal(w.Body.String(), "{\"wait\":\"1m30s\"}\n")
	w = post(t, route, "application/json", `{"wait":"soon"}`)
	is.Equal(w.Code, 422)
	is.Equal(w.Body.String(), `{"error":"wait is invalid","fields":{"wait":["is invalid"]}}`+"\n")
	// A plain time.Duration has no JSON format, which is the server's bug.
	type plain struct {
		Wait time.Duration `json:"wait"`
	}
	w = post(t, koi.New().Wrap(func(req *koi.Request[plain]) (res koi.Response[koi.Empty]) {
		return res
	}), "application/json", `{"wait":1500000000}`)
	is.Equal(w.Code, 500)
	w = serve(t, koi.New().Wrap(func(req *koi.Request[koi.Empty]) (res koi.Response[plain]) {
		return res.Body(plain{Wait: time.Second})
	}), "application/json")
	is.Equal(w.Code, 500)
}
