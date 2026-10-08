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
