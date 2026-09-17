package koi

import (
	"fmt"
	"net/http"
)

// Response is an outcome. Its zero value is an empty successful response.
// Builder methods return a copy, so return or assign their result.
type Response[Out any] struct {
	body     Out
	hasBody  bool
	view     string
	redirect string
	status   int
	header   http.Header
	err      error
	message  string
}

// Body sets the data to encode as JSON or render with the HTML view.
// A zero or nil value still counts as a body.
func (r Response[Out]) Body(value Out) Response[Out] {
	r.body = value
	r.hasBody = true
	return r
}

// Render sets the body and overrides the route’s HTML view.
// JSON responses encode value without using the view.
func (r Response[Out]) Render(name string, value Out) Response[Out] {
	r.view = name
	return r.Body(value)
}

// Redirect sets a 303 redirect, overriding the route’s redirect.
// With a body, only HTML redirects; JSON receives the body.
// Without a body, both representations redirect. Relative targets append to
// the request path; leading-slash and absolute targets are used directly.
// An empty target clears the override.
func (r Response[Out]) Redirect(target string) Response[Out] {
	r.redirect = target
	return r
}

// Error sets the failure outcome, which takes precedence over a body or redirect.
// Errors default to 500 unless Status is set. Details are hidden unless Koi.Debug
// is enabled. A nil error clears the failure.
func (r Response[Out]) Error(err error) Response[Out] {
	r.err = err
	return r
}

// Errorf formats an error with fmt.Errorf and sets the failure outcome.
// Like Error, details are hidden unless Koi.Debug is enabled. It supports %w wrapping.
func (r Response[Out]) Errorf(format string, args ...any) Response[Out] {
	return r.Error(fmt.Errorf(format, args...))
}

// Message sets a client-facing failure message while retaining err for logs.
func (r Response[Out]) Message(format string, args ...any) Response[Out] {
	r.message = fmt.Sprintf(format, args...)
	return r
}

// Status overrides the route’s normal response status or classifies an error.
// It does not change the 303 redirect status. Zero clears the override.
func (r Response[Out]) Status(code int) Response[Out] {
	r.status = code
	return r
}

// Header sets a response header, replacing any previous value for name.
// Koi manages Content-Type and preserves Vary: Accept during negotiation.
func (r Response[Out]) Header(name, value string) Response[Out] {
	r.header = r.header.Clone()
	if r.header == nil {
		r.header = make(http.Header)
	}
	r.header.Set(name, value)
	return r
}
