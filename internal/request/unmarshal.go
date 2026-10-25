package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/ajg/form"
)

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrDuplicateField       = errors.New("duplicate request field")
	ErrInvalidKey           = errors.New("invalid request key")
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
		// Compare root names, so tags.0=a in the query collides with tags in
		// the body.
		seen := make(map[string]bool, len(fields))
		for field := range fields {
			seen[rootName(field)] = true
		}
		for _, field := range slices.Sorted(maps.Keys(query)) {
			if seen[rootName(field)] {
				return fmt.Errorf("%w %q", ErrDuplicateField, field)
			}
		}
	}
	return nil
}

// rootName returns the lowercased top-level field name of a key. Decoding
// matches names case-insensitively, so duplicates must too.
func rootName(key string) string {
	if i := strings.IndexAny(key, ".["); i >= 0 {
		key = key[:i]
	}
	return strings.ToLower(key)
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
	case "multipart/form-data":
		return unmarshalMultipart(r, v)
	}
	if strings.HasSuffix(mediaType, "+json") {
		return unmarshalJSON(r.Body, v)
	}
	return nil, fmt.Errorf("%w %q", ErrUnsupportedMediaType, mediaType)
}

func unmarshalValues(v any, values url.Values) error {
	values, err := normalize(values)
	if err != nil {
		return err
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	return dec.DecodeValues(v, values)
}

// normalize rewrites bracket keys like tags[] and created_at[gte] into
// ajg/form's dot syntax. It copies because values may be the request's form.
func normalize(values url.Values) (url.Values, error) {
	normalized := make(url.Values, len(values))
	for key, vals := range values {
		key, err := bracketKey(key)
		if err != nil {
			return nil, err
		}
		normalized[key] = append(normalized[key], vals...)
	}
	return normalized, nil
}

var segmentEscaper = strings.NewReplacer(`\`, `\\`, ".", `\.`)

// bracketKey rewrites a[b][c] to a.b.c and a[] to a._
func bracketKey(key string) (string, error) {
	root, rest, ok := strings.Cut(key, "[")
	if !ok {
		return key, nil
	}
	var path strings.Builder
	path.WriteString(root)
	for {
		segment, after, ok := strings.Cut(rest, "]")
		if !ok {
			return "", fmt.Errorf("%w %q", ErrInvalidKey, key)
		}
		if segment == "" && after != "" {
			// ajg/form only indexes implicitly at the last level
			return "", fmt.Errorf("%w %q", ErrInvalidKey, key)
		}
		if segment == "" {
			segment = "_"
		}
		path.WriteString(".")
		path.WriteString(segmentEscaper.Replace(segment))
		if after == "" {
			return path.String(), nil
		}
		if after[0] != '[' {
			return "", fmt.Errorf("%w %q", ErrInvalidKey, key)
		}
		rest = after[1:]
	}
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

// maxMemory is the number of bytes of a multipart body held in memory. The
// remainder spills to temporary files.
const maxMemory = 32 << 20

// unmarshalMultipart decodes multipart values like form values and assigns
// uploaded files to top-level *multipart.FileHeader and []*multipart.FileHeader
// fields
func unmarshalMultipart(r *http.Request, v any) (url.Values, error) {
	if r.MultipartForm == nil {
		if err := r.ParseMultipartForm(maxMemory); err != nil {
			return nil, err
		}
	}
	values := url.Values(r.MultipartForm.Value)
	if err := unmarshalValues(v, values); err != nil {
		return nil, err
	}
	fields := make(url.Values, len(values)+len(r.MultipartForm.File))
	maps.Copy(fields, values)
	for key := range r.MultipartForm.File {
		fields[key] = nil
	}
	if err := unmarshalFiles(v, r.MultipartForm.File); err != nil {
		return nil, err
	}
	return fields, nil
}

var (
	fileHeaderType  = reflect.TypeFor[*multipart.FileHeader]()
	fileHeadersType = reflect.TypeFor[[]*multipart.FileHeader]()
)

func unmarshalFiles(v any, files map[string][]*multipart.FileHeader) error {
	if len(files) == 0 {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	for field := range rv.Type().Fields() {
		if !field.IsExported() || (field.Type != fileHeaderType && field.Type != fileHeadersType) {
			continue
		}
		name := fieldName(field)
		if name == "" {
			continue
		}
		headers := lookupFiles(files, name)
		if len(headers) == 0 {
			continue
		}
		if field.Type == fileHeaderType {
			rv.FieldByIndex(field.Index).Set(reflect.ValueOf(headers[0]))
		} else {
			rv.FieldByIndex(field.Index).Set(reflect.ValueOf(headers))
		}
	}
	return nil
}

// lookupFiles prefers an exact key match, then falls back to the first
// case-insensitive match in sorted key order so the result is deterministic
func lookupFiles(files map[string][]*multipart.FileHeader, name string) []*multipart.FileHeader {
	if headers, ok := files[name]; ok {
		return headers
	}
	for _, key := range slices.Sorted(maps.Keys(files)) {
		if strings.EqualFold(key, name) {
			return files[key]
		}
	}
	return nil
}

// fieldName resolves a field's name the same way as ajg/form: the form tag,
// then the json tag, then the field name. Returns an empty string for ignored
// fields.
func fieldName(field reflect.StructField) string {
	for _, key := range []string{"form", "json"} {
		tag, ok := field.Tag.Lookup(key)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
	}
	return field.Name
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
