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

// Unmarshal the request data into v
func Unmarshal(r *http.Request, v any) error {
	if r.Method == http.MethodGet {
		return unmarshalURL(r.URL, v)
	}
	bodyFields, err := unmarshalBody(r, v)
	if err != nil {
		return err
	}
	queryFields := r.URL.Query()
	for field := range queryFields {
		if _, ok := bodyFields[field]; ok {
			return fmt.Errorf("%w %q", ErrDuplicateField, field)
		}
	}
	return unmarshalURL(r.URL, v)
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

func unmarshalURL(u *url.URL, v any) error {
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	return dec.DecodeValues(v, u.Query())
}

func unmarshalForm(r *http.Request, v any) (url.Values, error) {
	if r.PostForm == nil {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	if err := dec.DecodeValues(v, r.PostForm); err != nil {
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
