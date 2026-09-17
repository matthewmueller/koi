package koi

import (
	"context"
	"html/template"
	"io"
)

// Renderer renders a named HTML view.
type Renderer interface {
	// Render writes the named view using the supplied data. Koi buffers output
	// and discards it if rendering fails.
	Render(ctx context.Context, w io.Writer, name string, data any) error
}

// htmlTemplate parses and executes a template file for each render.
type htmlTemplate struct{}

var _ Renderer = (*htmlTemplate)(nil)

func (v *htmlTemplate) Render(ctx context.Context, w io.Writer, name string, data any) error {
	t, err := template.ParseFiles(name)
	if err != nil {
		return err
	}
	return t.Execute(w, data)
}
