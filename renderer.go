package koi

import (
	"context"
	"html/template"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// View is a view and the props to render it with. It's an alias of an unnamed
// type, so a renderer in another package can declare the same type and
// implement Renderer without importing koi.
type View = struct {
	Name  string
	Props any
}

// Renderer renders HTML views.
type Renderer interface {
	// Render writes one HTML document from a stack of views, outermost first.
	// The last view is the page and the ones before it are its parents. Koi
	// buffers output and discards it if rendering fails.
	Render(ctx context.Context, w io.Writer, views ...View) error
}

// htmlTemplate parses and executes a template file for each view.
type htmlTemplate struct{}

var _ Renderer = (*htmlTemplate)(nil)

func (v *htmlTemplate) Render(ctx context.Context, w io.Writer, views ...View) error {
	// Render from the page outward, so each parent can place the HTML inside it.
	var inner template.HTML
	for i, view := range slices.Backward(views) {
		// Funcs must be registered before parsing a template that calls them.
		t, err := template.New(filepath.Base(view.Name)).Funcs(template.FuncMap{
			"slot": func() template.HTML { return inner },
		}).ParseFiles(view.Name)
		if err != nil {
			return err
		}
		// The outermost view is the document, so it's written without a copy.
		if i == 0 {
			return t.Execute(w, view.Props)
		}
		var html strings.Builder
		if err := t.Execute(&html, view.Props); err != nil {
			return err
		}
		inner = template.HTML(html.String())
	}
	return nil
}
