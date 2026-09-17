package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/ajg/form"
)

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrDuplicateField       = errors.New("duplicate request field")
)

// Unmarshal decodes query and body values using json and form tags.
// Routers should include path parameters in the query string.
func Unmarshal(r *http.Request, v any) error {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}
	if err := unmarshalValues(v, query); err != nil {
		return fmt.Errorf("decode query: %w", err)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fields, err := unmarshalBody(r, v)
		if err != nil {
			return err
		}
		for field := range query {
			if _, ok := fields[field]; ok {
				return fmt.Errorf("%w %q", ErrDuplicateField, field)
			}
		}
	}
	return nil
}

func unmarshalBody(r *http.Request, v any) (url.Values, error) {
	if r.ContentLength == 0 {
		return nil, nil
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return nil, ErrUnsupportedMediaType
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	switch mediaType {
	case "application/json":
		return unmarshalJSON(r.Body, v)
	case "application/x-www-form-urlencoded":
		return unmarshalForm(r, v)
	}
	if strings.HasSuffix(mediaType, "+json") {
		return unmarshalJSON(r.Body, v)
	}
	return nil, fmt.Errorf("%w %q", ErrUnsupportedMediaType, mediaType)
}

func unmarshalValues(v any, values url.Values) error {
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	return dec.DecodeValues(v, values)
}

func unmarshalForm(r *http.Request, v any) (url.Values, error) {
	if r.PostForm == nil {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
	}
	if err := unmarshalValues(v, r.PostForm); err != nil {
		return nil, err
	}
	return r.PostForm, nil
}

func unmarshalJSON(body io.Reader, v any) (url.Values, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	if err := dec.Decode(v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("request body must contain a single JSON value")
		}
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, nil
	}
	values := make(url.Values, len(fields))
	for field := range fields {
		values[field] = nil
	}
	return values, nil
}
