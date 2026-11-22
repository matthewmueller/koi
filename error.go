package koi

import (
	"encoding/json/v2"
	"net/http"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Invalid contains client-facing field validation failures.
// Validators return nil when input is valid.
type Invalid map[string][]string

// ResponseError contains the safe data passed to a custom error view.
type ResponseError struct {
	Status  int
	Message string
	Fields  Invalid
}

func (r *ResponseError) StatusText() string {
	if r.Status == 0 {
		return http.StatusText(http.StatusInternalServerError)
	}
	return http.StatusText(r.Status)
}

// StatusError wraps Err with a status and a client-facing message. Pass it to
// Response.Error, directly or wrapped, to choose the response status and
// message while keeping Err for logs and debug output.
type StatusError struct {
	Status  int
	Message string
	Err     error
}

var _ error = (*StatusError)(nil)

// Error returns the wrapped error's text, falling back to the message.
func (e *StatusError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func (e *StatusError) Unwrap() error {
	return e.Err
}

// BadRequest wraps err as a 400 with a client-facing message.
func BadRequest(err error, message string) error {
	return &StatusError{http.StatusBadRequest, message, err}
}

// Unauthorized wraps err as a 401 with a client-facing message.
func Unauthorized(err error, message string) error {
	return &StatusError{http.StatusUnauthorized, message, err}
}

// Forbidden wraps err as a 403 with a client-facing message.
func Forbidden(err error, message string) error {
	return &StatusError{http.StatusForbidden, message, err}
}

// NotFound wraps err as a 404 with a client-facing message.
func NotFound(err error, message string) error {
	return &StatusError{http.StatusNotFound, message, err}
}

// Conflict wraps err as a 409 with a client-facing message.
func Conflict(err error, message string) error {
	return &StatusError{http.StatusConflict, message, err}
}

var _ error = (Invalid)(nil)
var _ error = (*Invalid)(nil)

// Add appends a validation message for field.
func (e *Invalid) Add(field, message string) {
	if *e == nil {
		*e = Invalid{}
	}
	(*e)[field] = append((*e)[field], message)
}

// Error summarizes the invalid fields in sorted order.
func (e Invalid) Error() string {
	fields := make([]string, 0, len(e))
	for field := range e {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	if len(fields) == 0 {
		return "invalid fields"
	}
	messages := make([]string, 0, len(fields))
	for _, field := range fields {
		messages = append(messages, capitalize(field)+" "+joinMessages(e[field]))
	}
	return strings.Join(messages, ". ")
}

func capitalize(value string) string {
	r, size := utf8.DecodeRuneInString(value)
	if size == 0 {
		return value
	}
	return string(unicode.ToUpper(r)) + value[size:]
}

func joinMessages(messages []string) string {
	switch len(messages) {
	case 0:
		return ""
	case 1:
		return messages[0]
	case 2:
		return strings.Join(messages, " and ")
	default:
		return strings.Join(messages[:len(messages)-1], ", ") + ", and " + messages[len(messages)-1]
	}
}

// MarshalJSON encodes the validation summary and field messages.
func (e Invalid) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Error  string              `json:"error"`
		Fields map[string][]string `json:"fields"`
	}{e.Error(), e})
}
