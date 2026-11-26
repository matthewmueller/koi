package request

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/ajg/form"
)

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrDuplicateField       = errors.New("duplicate request field")
	ErrInvalidKey           = errors.New("invalid request key")
)

// Describe returns the response a decoding error deserves: its status, the
// field it's about when the decoder says, and a message. The message is empty
// unless the error is known to describe only the request, because other errors
// can describe the server: a read timeout names its address, a failed upload
// names a directory, and a form value of the wrong type prints the input's Go
// type.
func Describe(err error) (status int, field, message string) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return http.StatusRequestEntityTooLarge, "", ""
	}
	if errors.Is(err, ErrUnsupportedMediaType) {
		return http.StatusUnsupportedMediaType, "", err.Error()
	}
	if semantic, ok := errors.AsType[*json.SemanticError](err); ok {
		// Without a JSON kind the error isn't about a value that was sent. It's
		// about the input's Go type, like a channel, which is the server's bug.
		if semantic.JSONKind == 0 {
			return http.StatusInternalServerError, "", ""
		}
		// The message says what was sent, which is always true. What the Go
		// type expects isn't: a time is a struct that decodes from a string.
		// The decoder's own text isn't used because it names Go types.
		message = "can't be " + sent[semantic.JSONKind]
		if errors.Is(err, json.ErrUnknownName) {
			message = "isn't a known field"
		} else if semantic.Err != nil {
			// The JSON type fits, but the Go type rejected the value.
			message = "is invalid"
		}
		field = strings.Join(slices.Collect(semantic.JSONPointer.Tokens()), ".")
		// The value at the root isn't a field, so the message names it.
		if field == "" {
			message = "body " + message
		}
		return http.StatusUnprocessableEntity, field, message
	}
	_, syntax := errors.AsType[*jsontext.SyntacticError](err)
	_, escape := errors.AsType[url.EscapeError](err)
	if syntax || escape || errors.Is(err, ErrDuplicateField) || errors.Is(err, ErrInvalidKey) {
		message = err.Error()
	}
	return http.StatusBadRequest, "", message
}

// sent names the kinds of JSON value in a message.
var sent = map[jsontext.Kind]string{
	jsontext.KindString:      "a string",
	jsontext.KindNumber:      "a number",
	jsontext.KindTrue:        "a boolean",
	jsontext.KindFalse:       "a boolean",
	jsontext.KindBeginArray:  "a list",
	jsontext.KindBeginObject: "an object",
}

// wildcard matches the name of a wildcard in a pattern, like id in {id},
// {id...} or the {id:[0-9]+} that other routers accept.
var wildcard = regexp.MustCompile(`\{([\pL\pN_]+)`)

// Unmarshal decodes path, query and body values using json and form tags.
// The options apply to JSON bodies.
func Unmarshal(r *http.Request, v any, opts ...json.Options) error {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}
	// The standard mux keeps path values out of the query. They replace every
	// query key for the same field, like ID or id[], so the query can't
	// override the path.
	for _, match := range wildcard.FindAllStringSubmatch(r.Pattern, -1) {
		maps.DeleteFunc(query, func(key string, _ []string) bool { return sameField(key, match[1]) })
		query.Set(match[1], r.PathValue(match[1]))
	}
	if err := unmarshalValues(v, query); err != nil {
		return fmt.Errorf("decode query: %w", err)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fields, err := unmarshalBody(r, v, opts...)
		if err != nil {
			return err
		}
		for _, field := range slices.Sorted(maps.Keys(query)) {
			for name := range fields {
				if sameField(field, name) {
					return fmt.Errorf("%w %q", ErrDuplicateField, field)
				}
			}
		}
	}
	return nil
}

// separators are ignored when JSON decoding matches a name to a field.
var separators = strings.NewReplacer("_", "", "-", "")

// sameField reports whether two keys can decode into the same top-level field,
// so tags.0 collides with tags. It has to match as loosely as the decoders do,
// or a body could set a field that the path or query already bound: in any
// case, and ignoring the dashes and underscores that JSON decoding ignores.
func sameField(a, b string) bool {
	return strings.EqualFold(separators.Replace(rootName(a)), separators.Replace(rootName(b)))
}

// rootName returns the top-level field name of a key.
func rootName(key string) string {
	if i := strings.IndexAny(key, ".["); i >= 0 {
		key = key[:i]
	}
	return key
}

func unmarshalBody(r *http.Request, v any, opts ...json.Options) (url.Values, error) {
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
		return unmarshalJSON(r.Body, v, opts...)
	case "application/x-www-form-urlencoded":
		return unmarshalForm(r, v)
	case "multipart/form-data":
		return unmarshalMultipart(r, v)
	}
	if strings.HasSuffix(mediaType, "+json") {
		return unmarshalJSON(r.Body, v, opts...)
	}
	return nil, fmt.Errorf("%w %q", ErrUnsupportedMediaType, mediaType)
}

func unmarshalValues(v any, values url.Values) error {
	values, err := normalize(v, values)
	if err != nil {
		return err
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	return dec.DecodeValues(v, values)
}

// normalize rewrites keys into ajg/form's dot syntax. Bracket keys like
// tags[] and created_at[gte] become tags._ and created_at.gte, and bare keys
// of top-level slice fields become implicitly indexed, so tags=a&tags=b
// decodes like tags._=a&tags._=b. It copies because values may be the
// request's form.
func normalize(v any, values url.Values) (url.Values, error) {
	sliceKeys := sliceFields(v)
	normalized := make(url.Values, len(values))
	for key, vals := range values {
		key, err := bracketKey(key)
		if err != nil {
			return nil, err
		}
		if sliceKeys[strings.ToLower(key)] {
			key += "._"
		}
		normalized[key] = append(normalized[key], vals...)
	}
	return normalized, nil
}

// sliceFields returns the lowercased names of top-level slice fields, other
// than byte slices which decode from a single string
func sliceFields(v any) map[string]bool {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	names := map[string]bool{}
	for field := range t.Fields() {
		if !field.IsExported() || field.Type.Kind() != reflect.Slice || field.Type.Elem().Kind() == reflect.Uint8 {
			continue
		}
		if name := fieldName(field); name != "" {
			names[strings.ToLower(name)] = true
		}
	}
	return names
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

func unmarshalJSON(body io.Reader, v any, opts ...json.Options) (url.Values, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	// A null body is no body. Decoding it would clear what the path and query
	// bound.
	if len(payload) == 0 || string(bytes.TrimSpace(payload)) == "null" {
		return nil, nil
	}
	// Names match in any case, like query and form keys do. JSON also ignores
	// dashes and underscores.
	if err := json.Unmarshal(payload, v, json.MatchCaseInsensitiveNames(true), json.JoinOptions(opts...)); err != nil {
		return nil, err
	}
	// Only an object has fields that can collide with the query.
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, nil
	}
	values := make(url.Values, len(fields))
	for field := range fields {
		values[field] = nil
	}
	return values, nil
}
