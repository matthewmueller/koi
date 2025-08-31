package invalid

import (
	"encoding/json"
	"errors"
	"sort"
)

var ErrInvalid = errors.New("invalid fields")

type Error struct {
	Fields  map[string][]string `json:"fields,omitempty"`
	Initial error               `json:"initial,omitempty"`
}

func (e *Error) Error() string {
	var list []string
	for field := range e.Fields {
		list = append(list, field)
	}
	if e.Initial != nil {
		list = append(list, e.Initial.Error())
	}
	return "invalid " + joinStrings(list)
}

func (e *Error) Unwrap() error {
	return ErrInvalid
}

func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(errorState{
		Error:  e.Error(),
		Fields: e.Fields,
	})
}

type errorState struct {
	Error  string              `json:"error"`
	Fields map[string][]string `json:"fields,omitzero"`
}

func Join(err error, field string, msg string) error {
	if ie, ok := err.(*Error); ok {
		ie.Fields[field] = append(ie.Fields[field], msg)
		return ie
	}
	ie := &Error{
		Fields: map[string][]string{field: {msg}},
	}
	// Add the initial error
	if err != nil {
		ie.Initial = err
	}
	return ie
}

func joinStrings(fields []string) string {
	sort.Strings(fields)
	if len(fields) == 1 {
		return fields[0]
	}
	if len(fields) == 2 {
		return fields[0] + " and " + fields[1]
	}
	s := ""
	for i, field := range fields {
		if i == len(fields)-1 {
			s += "and " + field
		} else {
			s += field + ", "
		}
	}
	return s
}
