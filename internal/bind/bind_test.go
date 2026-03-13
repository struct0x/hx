package bind

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type CustomValue struct {
	Value string
}

func (c CustomValue) UnmarshalText(text []byte) error {
	c.Value = string(text)

	return nil
}

func TestBind(t *testing.T) {
	type testCase struct {
		name  string
		setup func() *http.Request
		opts  []Opt
		run   func(t *testing.T, req *http.Request, opts []Opt)
	}

	tests := []testCase{
		{
			name: "nil_request",
			setup: func() *http.Request {
				return nil
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct{}
				err := Bind(nil, nil, opts...)
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if !errors.Is(err, ErrNilRequest) {
					t.Errorf("expected error to be ErrNilRequest, got %v", err)
				}
			},
		},
		{
			name: "nil_target",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct{}
				err := Bind(req, nil, opts...)
				if err == nil {
					t.Errorf("expected error, got nil")
				}

				if !errors.Is(err, ErrNilDestination) {
					t.Errorf("expected error to be ErrNilDestination, got %v", err)
				}
			},
		},
		{
			name: "not_a_struct",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				var dst int

				err := Bind(req, &dst, opts...)
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if !errors.Is(err, ErrNotAStruct) {
					t.Errorf("expected error to be ErrNotAStruct, got %v", err)
				}
			},
		},
		{
			name: "all_sources",
			setup: func() *http.Request {
				body := strings.NewReader(`{"name":"John Doe","age":30, "amount": 123.45}`)
				req := httptest.NewRequest(http.MethodPost, "/users/usr-123?tags=go&tags=web&surname=doe&age=123", body)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Mozilla/5.0")
				req.AddCookie(&http.Cookie{Name: "session_id", Value: "abc123"})
				return req
			},
			opts: []Opt{
				WithPathValueFunc(func(r *http.Request, name string) string {
					if name == "id" {
						return "usr-123"
					}
					return ""
				}),
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					ID      string      `path:"id" validate:"required"`
					Name    string      `json:"name" validate:"required"`
					Surname *string     `query:"surname"`
					Age     uint        `json:"age" validate:"required,min=18"`
					AgeQ    uint        `query:"age"`
					Amount  *float64    `json:"amount"`
					Tags    []string    `query:"tags" validate:"required,len=2"`
					Custom  CustomValue `json:"custom"`
					Agent   string      `header:"User-Agent"`
					Session string      `cookie:"session_id"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				amount := 123.45
				surname := "doe"

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("ID", "usr-123"),
					checkField[target]("Name", "John Doe"),
					checkField[target]("Surname", &surname),
					checkField[target]("Age", uint(30)),
					checkField[target]("AgeQ", uint(123)),
					checkField[target]("Amount", &amount),
					checkSliceField[target]("Tags", []string{"go", "web"}),
					checkField[target]("Agent", "Mozilla/5.0"),
					checkField[target]("Session", "abc123"),
				)
			},
		},
		{
			name: "query_params_single_and_array",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/search?q=golang&limit=10&limit=12&tags=web&tags=api", nil)
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Query string   `query:"q"`
					Limit []int    `query:"limit"`
					Tags  []string `query:"tags"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Query", "golang"),
					checkField[target]("Limit", []int{10, 12}),
					checkSliceField[target]("Tags", []string{"web", "api"}),
				)
			},
		},
		{
			name: "json_body_nested",
			setup: func() *http.Request {
				body := strings.NewReader(`{"user":{"password": "abc", "nested": {"password": "bcd"}},"score":42,"active":true}`)
				req := httptest.NewRequest(http.MethodPost, "/", body)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type nested struct {
					Password string  `json:"password"`
					Nested   *nested `json:"nested"`
				}
				type target struct {
					User   nested `json:"user"`
					Score  int    `json:"score"`
					Active bool   `json:"active"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("User", nested{Password: "abc", Nested: &nested{Password: "bcd"}}),
					checkField[target]("Score", 42),
					checkField[target]("Active", true),
				)
			},
		},
		{
			name: "headers_various_types",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("X-Request-ID", "req-456")
				req.Header.Set("X-Rate-Limit", "100")
				req.Header.Set("Authorization", "Bearer token123")
				req.Header.Add("X-Forwarded-For", "127.0.0.1")
				req.Header.Add("X-Forwarded-For", "192.168.1.1")
				req.Header["X-Empty"] = []string{}
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					RequestID     string   `header:"X-Request-ID"`
					RateLimit     int      `header:"X-Rate-Limit"`
					Auth          string   `header:"Authorization"`
					XForwardedFor []string `header:"X-Forwarded-For"`
					XEmpty        []string `header:"X-Empty"`
					XPtr          *string  `header:"X-Ptr"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("RequestID", "req-456"),
					checkField[target]("RateLimit", 100),
					checkField[target]("Auth", "Bearer token123"),
					checkSliceField[target]("XForwardedFor", []string{"127.0.0.1", "192.168.1.1"}),
					checkSliceField[target]("XEmpty", []string{}),
					checkField[target]("XPtr", (*string)(nil)),
				)
			},
		},
		{
			name: "no_cookie",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Session string `cookie:"session"`
				}

				var dst target
				err := Bind(req, &dst, opts...)
				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Session", ""),
				)
			},
		},
		{
			name: "cookies_multiple",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.AddCookie(&http.Cookie{Name: "session", Value: "sess-xyz"})
				req.AddCookie(&http.Cookie{Name: "user_id", Value: "42"})
				req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Session string       `cookie:"session"`
					UserID  int          `cookie:"user_id"`
					Theme   *http.Cookie `cookie:"theme"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Session", "sess-xyz"),
					checkField[target]("UserID", 42),
					checkField[target]("Theme", &http.Cookie{Name: "theme", Value: "dark"}),
				)
			},
		},
		{
			name: "path_params",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/users/42/posts/99", nil)
			},
			opts: []Opt{
				WithPathValueFunc(func(r *http.Request, name string) string {
					vals := map[string]string{
						"user_id": "42",
						"post_id": "99",
					}
					return vals[name]
				}),
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					UserID int    `path:"user_id"`
					PostID string `path:"post_id"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("UserID", 42),
					checkField[target]("PostID", "99"),
				)
			},
		},
		{
			name: "multipart_file_upload",
			setup: func() *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				part, _ := writer.CreateFormFile("avatar", "profile.jpg")
				part.Write([]byte("fake-image-data"))

				writer.WriteField("description", "My avatar")
				writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/upload", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Avatar []*multipart.FileHeader `file:"avatar"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkFieldNotNil[target]("Avatar"),
				)
			},
		},
		{
			name: "type_conversion_numeric",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/items?count=42&price=19.99&available=true", nil)
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Count     int     `query:"count"`
					Price     float64 `query:"price"`
					Available bool    `query:"available"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Count", 42),
					checkField[target]("Price", 19.99),
					checkField[target]("Available", true),
				)
			},
		},
		{
			name: "invalid_type_conversion",
			setup: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/items?count=invalid", nil)
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Count int `query:"count"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkHasError[target],
				)
			},
		},
		{
			name: "empty_json_body",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Name string `json:"name"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Name", ""),
				)
			},
		},
		{
			name: "malformed_json",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name": invalid`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Name string `json:"name"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkHasError[target],
				)
			},
		},
		{
			name: "max_form_memory",
			setup: func() *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				part, _ := writer.CreateFormFile("file", "large.bin")
				part.Write(bytes.Repeat([]byte("x"), 1*1024*1024)) // 1MB
				writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			opts: []Opt{
				WithMaxFormMemoryMB(2),
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					File []*multipart.FileHeader `file:"file"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkFieldNotNil[target]("File"),
				)
			},
		},
		{
			name: "embedded_struct",
			setup: func() *http.Request {
				body := strings.NewReader(`{"name":"Bob","age":25}`)
				req := httptest.NewRequest(http.MethodPost, "/users?active=true", body)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Request-ID", "req-789")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					CommonFields
					Name string `json:"name"`
					Age  int    `json:"age"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Name", "Bob"),
					checkField[target]("Age", 25),
					checkField[target]("Active", true),
					checkField[target]("RequestID", "req-789"),
				)
			},
		},
		{
			name: "pointer_fields",
			setup: func() *http.Request {
				body := strings.NewReader(`{"count":42}`)
				req := httptest.NewRequest(http.MethodPost, "/", body)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Count *int `json:"count"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkPointerField[target]("Count", 42),
				)
			},
		},
		{
			name: "case_sensitive_headers",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					ContentType string `header:"content-type"`
				}
				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("ContentType", "application/json"),
				)
			},
		},
		{
			name: "validation_error",
			setup: func() *http.Request {
				body := strings.NewReader(`{"name":"Bob","age":-1}`)
				req := httptest.NewRequest(http.MethodPost, "/users?active=true", body)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Request-ID", "req-789")
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					CommonFields
					Name string `json:"name" validate:"required"`
					Age  int    `json:"age" validate:"min=0"`
				}

				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkHasError[target],
					checkField[target]("Name", "Bob"),
					checkField[target]("Age", -1),
					checkField[target]("Active", true),
					checkField[target]("RequestID", "req-789"),
				)
			},
		},
		{
			name: "custom_target",
			setup: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/users?name=Bob", nil)
				return req
			},
			run: func(t *testing.T, req *http.Request, opts []Opt) {
				type target struct {
					Name CustomTarget `query:"name"`
				}

				var dst target
				err := Bind(req, &dst, opts...)

				checkAll(t, &dst, err,
					checkNoError[target],
					checkField[target]("Name", CustomTarget{Name: "Bob"}),
					func(t *testing.T, dst *target, err error) {
						if !dst.Name.called {
							t.Errorf("expected UnmarshalText to be called")
						}
					},
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()
			tt.run(t, req, tt.opts)
		})
	}
}

type CustomTarget struct {
	Name   string
	called bool
}

func (c *CustomTarget) UnmarshalText(text []byte) error {
	c.Name = string(text)
	c.called = true
	return nil
}

// CommonFields is used for testing embedded structs
type CommonFields struct {
	Active    bool   `query:"active"`
	RequestID string `header:"X-Request-ID"`
}

// Check functions with generics

type checkFunc[T any] func(t *testing.T, dst *T, err error)

func checkNoError[T any](t *testing.T, dst *T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func checkHasError[T any](t *testing.T, dst *T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func checkField[T any](fieldName string, expected any) func(t *testing.T, dst *T, err error) {
	return func(t *testing.T, dst *T, err error) {
		t.Helper()
		actual := getFieldValue(t, dst, fieldName)
		opts := cmp.Options{}
		if reflect.TypeOf(expected).Kind() == reflect.Struct {
			opts = append(opts, cmpopts.IgnoreUnexported(expected))
		}

		if diff := cmp.Diff(expected, actual, opts...); diff != "" {
			t.Errorf("field %q: expected %v, got %v (-want +got):\n%s", fieldName, expected, actual, diff)
		}
	}
}

func checkSliceField[T any](fieldName string, expected []string) func(t *testing.T, dst *T, err error) {
	return func(t *testing.T, dst *T, err error) {
		t.Helper()
		actual := getFieldValue(t, dst, fieldName)

		act, ok := actual.([]string)
		if !ok {
			t.Errorf("field %q: expected []string, got %T", fieldName, actual)
			return
		}
		if len(act) != len(expected) {
			t.Errorf("field %q: expected %v, got %v", fieldName, expected, act)
			return
		}
		for i := range expected {
			if act[i] != expected[i] {
				t.Errorf("field %q[%d]: expected %q, got %q", fieldName, i, expected[i], act[i])
			}
		}
	}
}

func checkFieldNotNil[T any](fieldName string) func(t *testing.T, dst *T, err error) {
	return func(t *testing.T, dst *T, err error) {
		t.Helper()
		v := reflect.ValueOf(dst).Elem()
		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			t.Fatalf("field %q not found in struct", fieldName)
		}
		if field.IsNil() {
			t.Errorf("field %q: expected non-nil value", fieldName)
		}
	}
}

func checkPointerField[T any](fieldName string, expected int) func(t *testing.T, dst *T, err error) {
	return func(t *testing.T, dst *T, err error) {
		t.Helper()
		actual := getFieldValue(t, dst, fieldName)

		if ptr, ok := actual.(*int); ok {
			if ptr == nil {
				t.Errorf("field %q: expected non-nil pointer", fieldName)
			} else if *ptr != expected {
				t.Errorf("field %q: expected *%v, got *%v", fieldName, expected, *ptr)
			}
		} else {
			t.Errorf("field %q: expected *int, got %T", fieldName, actual)
		}
	}
}

func checkAll[T any](t *testing.T, dst *T, err error, ch ...checkFunc[T]) {
	t.Helper()
	for _, check := range ch {
		check(t, dst, err)
	}
}

func getFieldValue[T any](t *testing.T, dst *T, fieldName string) any {
	t.Helper()

	v := reflect.ValueOf(dst).Elem()
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("field %q not found in struct", fieldName)
	}

	return field.Interface()
}

func TestMultipleTagsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?name=alice", nil)

	type invalidStruct struct {
		Name string `query:"name" json:"name"` // Multiple bind tags - should error
	}

	var dst invalidStruct
	err := Bind(req, &dst)

	if !errors.Is(err, ErrMultipleTags) {
		t.Fatalf("expected error to be ErrMultipleTags, got: %v", err)
	}
}

func TestEmptyTagError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?name=alice", nil)

	type invalidStruct struct {
		Name string `query:""` // Empty bind tag - should error
	}

	var dst invalidStruct
	err := Bind(req, &dst)

	if err == nil {
		t.Fatal("expected error for field with multiple bind tags, got nil")
	}

	if !errors.Is(err, ErrEmptyTag) {
		t.Fatalf("expected error to be ErrEmptyTag, got: %v", err)
	}
}

func TestMultipleBinds(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?name=alice", nil)
	req.Header.Set("Content-Type", "application/json")

	type target struct {
		Name string `query:"name"`
	}

	var dst target
	err := Bind(req, &dst)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	err = Bind(req, &dst)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSliceOfPointers(t *testing.T) {
	type Request struct {
		IDs []*int `query:"ids"`
	}

	dst := &Request{}
	req := httptest.NewRequest(http.MethodGet, "/?ids=1&ids=2", nil)

	err := Bind(req, dst)

	// This WILL return "slice of pointers not supported" error
	// from setSliceFromStrings at line 449 in bind.go
	if err == nil {
		t.Fatal("Expected error for slice of pointers, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestFile(t *testing.T) {
	// Build a multipart body *once* – the same body can be reused because
	// the binder only reads the headers, not the file contents.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fw, err := w.CreateFormFile("avatar", "avatar.jpg")
	if err != nil {
		t.Fatalf("multipart create error: %v", err)
	}
	// small dummy payload – the actual bytes are irrelevant for the binder.
	_, _ = io.Copy(fw, strings.NewReader("dummy-data"))
	w.Close()

	contentType := w.FormDataContentType()

	buf := bytes.NewReader(body.Bytes())

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Type", contentType)

	var dst struct {
		Avatar *multipart.FileHeader `file:"avatar"`
	}

	if err := Bind(req, &dst); err != nil {
		t.Fatalf("bind error: %v", err)
	}

	if dst.Avatar.Header == nil {
		t.Fatal("expected non-nil Header")
	}

	if dst.Avatar.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("expected Content-Type to be application/octet-stream got %s", dst.Avatar.Header.Get("Content-Type"))
	}
}
