package koi

import "net/http"

// Request provides decoded input alongside the original HTTP request.
type Request[In any] struct {
	*http.Request
	// Input contains bound and validated path, query and body values. Path
	// values come from the standard mux's wildcards, like {id}, or from the
	// query string for routers that put them there.
	Input In
}

// Accepts reports whether the Accept header allows html, json, or a media type.
// It respects quality values and wildcards; a missing header accepts any type.
// Acceptability does not imply that this representation will be selected.
func (r *Request[In]) Accepts(format string) bool {
	return quality(r.Header.Get("Accept"), mediaType(format)) > 0
}
