package bind

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	ErrNilRequest     = errors.New("bind: request is nil")
	ErrNilDestination = errors.New("bind: destination is nil")
	ErrNotAStruct     = errors.New("bind: destination must be a struct")
	ErrExpectedStruct = errors.New("bind: expected struct")
	ErrMultipleTags   = errors.New("bind: multiple tags")
	ErrEmptyTag       = errors.New("bind: tag is empty")
)

// Opt is a functional option that configures the binder.
type Opt interface {
	BindOpt()
}

type withPathValueFunc func(r *http.Request, name string) string

func (f withPathValueFunc) BindOpt() {}

func WithPathValueFunc(fn func(r *http.Request, name string) string) Opt {
	return withPathValueFunc(fn)
}

// defaultPathValue is the fallback implementation that uses
// http.Request.PathValue (available since Go 1.20).
func defaultPathValue(r *http.Request, name string) string {
	return r.PathValue(name)
}

type maxFormMemory int64

func (m maxFormMemory) BindOpt() {}

func WithMaxFormMemoryMB(maxFormMemoryMB int64) Opt {
	return maxFormMemory(maxFormMemoryMB * 1024 * 1024)
}

type ivalidator struct {
	v *validator.Validate
}

func (m ivalidator) BindOpt() {}

func WithValidator(v *validator.Validate) Opt {
	return ivalidator{v}
}

var defaultValidator = validator.New()

func Bind(r *http.Request, dst any, opts ...Opt) error {
	if r == nil {
		return ErrNilRequest
	}
	if dst == nil {
		return ErrNilDestination
	}

	maxFormMemoryBytes := int64(32 << 20)
	pathValueFunc := defaultPathValue
	valid := defaultValidator
	for _, opt := range opts {
		switch o := opt.(type) {
		case withPathValueFunc:
			pathValueFunc = o
		case maxFormMemory:
			maxFormMemoryBytes = int64(o)
		case ivalidator:
			valid = o.v
		}
	}

	structVal := reflect.ValueOf(dst).Elem()
	if structVal.Kind() != reflect.Struct {
		return fmt.Errorf("%w: %T", ErrNotAStruct, dst)
	}

	bindErrs := new(Errors)

	structType := structVal.Type()
	fields, err := cachedFieldInfo(structType)
	if err != nil {
		return err
	}

	for i := range fields {
		fields[i].value = getFieldByPath(structVal, fields[i].fieldIdx)
	}

	needJSON := false
	needMultipart := false
	needQuery := false
	for _, f := range fields {
		switch f.src {
		case srcQuery:
			needQuery = true
		case srcJSON:
			needJSON = true
		case srcFile:
			needMultipart = true
		default:
			continue
		}
	}

	var jsonMap *map[string]json.RawMessage
	if needJSON && isJSONContent(r.Header.Get("Content-Type")) {
		m := make(map[string]json.RawMessage)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&m); err != nil && err != io.EOF {
			bindErrs.Append(Error{
				Field: "<json-body>",
				Err:   fmt.Errorf("failed to decode JSON body with \"%w\"", err),
			})
		} else {
			jsonMap = &m
		}
	}
	if needMultipart && isMultipartContent(r.Header.Get("Content-Type")) {
		if err := r.ParseMultipartForm(maxFormMemoryBytes); err != nil && !errors.Is(err, http.ErrNotMultipart) && !errors.Is(err, io.EOF) {
			bindErrs.Append(Error{
				Field: "<multipart-form>",
				Err:   fmt.Errorf("failed to parse multipart form with \"%w\"", err),
			})
		}
	}

	var query url.Values
	if needQuery {
		q := r.URL.Query()
		query = q
	}

	for _, f := range fields {
		if err := bindField(r, pathValueFunc, f, jsonMap, query); err != nil {
			bindErrs.Append(Error{
				Field: f.tagValue,
				Err:   err,
			})
		}
	}

	if len(bindErrs.Errors) > 0 {
		return bindErrs
	}

	if err := valid.Struct(dst); err != nil {
		return convIntoBindErrs(err)
	}

	return nil
}

func convIntoBindErrs(err error) error {
	var validationErr validator.ValidationErrors
	if errors.As(err, &validationErr) {
		bindErrs := new(Errors)

		for _, field := range validationErr {
			bindErrs.Append(Error{
				Field: field.Field(),
				Err:   field,
			})
		}

		return bindErrs
	}

	return err
}

// isJSONContent reports whether the supplied Content‑Type denotes JSON.
func isJSONContent(mediaType string) bool {
	return strings.HasPrefix(mediaType, "application/json")
}

// isMultipartContent reports whether the supplied Content‑Type denotes a multipart request.
func isMultipartContent(mediaType string) bool {
	return strings.HasPrefix(mediaType, "multipart/")
}

func bindField(
	r *http.Request,
	pathValueFunc func(*http.Request, string) string,
	fi fieldInfo,
	jsonMap *map[string]json.RawMessage,
	query url.Values,
) error {
	switch fi.src {
	case srcQuery:
		return bindFromValues(query, fi.tagValue, fi.value)
	case srcHeader:
		return bindFromHeader(r.Header, fi.tagValue, fi.value)
	case srcCookie:
		return bindFromCookie(r, fi.tagValue, fi.value)
	case srcPath:
		return bindFromPath(pathValueFunc, r, fi.tagValue, fi.value)
	case srcJSON:
		return bindFromJSON(jsonMap, fi.tagValue, fi.value)
	case srcFile:
		return bindFromFile(r, fi.tagValue, fi.value)
	default:
		return fmt.Errorf("unsupported source %v", fi.src)
	}
}

func bindFromValues(vals url.Values, name string, dst reflect.Value) error {
	src := vals.Get(name)
	if src == "" {
		return nil
	}

	if dst.CanAddr() {
		addr := dst.Addr()
		if u, ok := addr.Interface().(encoding.TextUnmarshaler); ok {
			return u.UnmarshalText([]byte(src))
		}
	}

	if dst.Kind() == reflect.Slice {
		return setSliceFromStrings(vals[name], dst)
	}

	return setFromString(src, dst)
}

func bindFromJSON(jsonMap *map[string]json.RawMessage, name string, dst reflect.Value) error {
	if jsonMap == nil || len(*jsonMap) == 0 {
		return nil
	}
	raw, ok := (*jsonMap)[name]
	if !ok {
		return nil
	}

	return json.Unmarshal(raw, dst.Addr().Interface())
}

func bindFromHeader(h http.Header, name string, dst reflect.Value) error {
	if dst.Kind() == reflect.Slice {
		strs := h.Values(name)
		if len(strs) == 0 {
			return nil
		}
		return setSliceFromStrings(strs, dst)
	}
	s := h.Get(name)
	if s == "" {
		return nil
	}
	return setFromString(s, dst)
}

func bindFromCookie(r *http.Request, name string, dst reflect.Value) error {
	c, err := r.Cookie(name)
	if err != nil {
		return nil
	}

	if dst.Type() == reflect.TypeOf(&http.Cookie{}) {
		dst.Set(reflect.ValueOf(c))
		return nil
	}
	return setFromString(c.Value, dst)
}

func bindFromPath(pvf func(r *http.Request, name string) string, r *http.Request, name string, dst reflect.Value) error {
	s := pvf(r, name)
	if s == "" {
		return nil
	}
	return setFromString(s, dst)
}

func bindFromFile(r *http.Request, name string, dst reflect.Value) error {
	if r.MultipartForm == nil {
		return nil
	}
	files := r.MultipartForm.File[name]
	if len(files) == 0 {
		return nil
	}

	if dst.Type() == reflect.TypeOf(&multipart.FileHeader{}) {
		if len(files) > 1 {
			return fmt.Errorf("multiple files for single file field %s", name)
		}
		dst.Set(reflect.ValueOf(files[0]))
		return nil
	}
	if dst.Type() == reflect.TypeOf([]*multipart.FileHeader{}) {
		slice := reflect.MakeSlice(dst.Type(), len(files), len(files))
		for i, fh := range files {
			slice.Index(i).Set(reflect.ValueOf(fh))
		}
		dst.Set(slice)
		return nil
	}
	return fmt.Errorf("unsupported file field type %s", dst.Type())
}

func setFromString(s string, dst reflect.Value) error {
	if !dst.CanSet() {
		return fmt.Errorf("cannot set value for %s", dst.Type())
	}

	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	switch dst.Kind() {
	case reflect.String:
		dst.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		dst.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := strconv.ParseUint(s, 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetFloat(f)
	default:
		return fmt.Errorf("unsupported destination kind %s", dst.Kind())
	}
	return nil
}

func setSliceFromStrings(strs []string, dst reflect.Value) error {
	if !dst.CanSet() {
		return fmt.Errorf("cannot set slice for %s", dst.Type())
	}
	elemKind := dst.Type().Elem().Kind()
	slice := reflect.MakeSlice(dst.Type(), len(strs), len(strs))
	for i, s := range strs {
		elem := slice.Index(i)
		if elemKind == reflect.Ptr {
			return fmt.Errorf("slice of pointers not supported")
		}

		if err := setFromString(s, elem); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}
	dst.Set(slice)
	return nil
}
