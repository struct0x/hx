package hx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/struct0x/crum"

	"github.com/struct0x/hx"
)

func TestHX(t *testing.T) {
	type check func(t *testing.T, wr *httptest.ResponseRecorder)
	checks := func(ch ...check) []check {
		return ch
	}

	hasStatus := func(status int) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			if wr.Code != status {
				t.Errorf("expected status %d, got %d", status, wr.Code)
			}
		}
	}

	hasBody := func(body string) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			if strings.TrimSpace(wr.Body.String()) != body {
				t.Errorf("expected body %q, got %q", body, wr.Body.String())
			}
		}
	}

	hasProblemDetails := func(details hx.ProblemDetails) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			var pd hx.ProblemDetails
			if err := json.Unmarshal(wr.Body.Bytes(), &pd); err != nil {
				t.Errorf("expected problem details, got: %v", err)
				return
			}
			if diff := cmp.Diff(details, pd); diff != "" {
				t.Errorf("problem details mismatch (-want +got):\n%s", diff)
			}
		}
	}

	hasHeaders := func(headers http.Header) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			for key, expected := range headers {
				got := wr.Header().Get(key)
				if got != expected[0] {
					t.Errorf("header %q: expected %q, got %q", key, expected[0], got)
				}
			}
		}
	}

	hasCookie := func(name, value string) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			cookies := wr.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatalf("expected cookie %q, got none", name)
			}
			if cookies[0].Name != name {
				t.Errorf("expected cookie %q, got %q", name, cookies[0].Name)
			}

			if cookies[0].Raw != value {
				t.Errorf("expected cookie %q, got %q", value, cookies[0].Raw)
			}
		}
	}

	tests := []struct {
		name    string
		handler hx.HandlerFunc
		method  string
		body    io.Reader
		checks  []check
	}{
		{
			name: "ok_response_with_string",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK("success")
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"success"`),
			),
		},
		{
			name: "response_io_reader",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK(strings.NewReader("success"))
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`success`),
			),
		},
		{
			name: "no_content_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.NoContent()
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusNoContent),
				hasBody(``),
			),
		},
		{
			name: "unknown_error",
			handler: func(ctx context.Context, r *http.Request) error {
				return errors.New("unknown error")
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusInternalServerError),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusInternalServerError,
					Instance:   "inst-abc123",
					Title:      "internal server error",
				}),
			),
		},
		{
			name: "ok_response_with_json_object",
			handler: func(ctx context.Context, r *http.Request) error {
				data := map[string]interface{}{
					"id":   123,
					"name": "John Doe",
				}
				return hx.OK(data)
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`{"id":123,"name":"John Doe"}`),
			),
		},
		{
			name: "bad_request_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.BadRequest(
					"invalid input",
					hx.WithCookie(crum.NewCookie("k", "v").MustBuild()),
				)
			},
			method: "POST",
			checks: checks(
				hasStatus(http.StatusBadRequest),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusBadRequest,
					Title:      "invalid input",
					Instance:   "inst-abc123",
				}),
			),
		},
		{
			name: "not_found_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.NotFound(
					"resource not found",
					hx.WithHeader("X-Custom-Header", "test-value"),
				)
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusNotFound),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusNotFound,
					Title:      "resource not found",
					Instance:   "inst-abc123",
				}),
			),
		},
		{
			name: "unauthorized_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.Unauthorized("access denied")
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusUnauthorized),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusUnauthorized,
					Title:      "access denied",
					Instance:   "inst-abc123",
				}),
			),
		},
		{
			name: "response_with_custom_header",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK("data", hx.WithHeader("X-Custom-Header", "test-value"))
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"data"`),
				hasHeaders(http.Header{
					"X-Custom-Header": []string{"test-value"},
				}),
			),
		},
		{
			name: "response_with_multiple_headers",
			handler: func(ctx context.Context, r *http.Request) error {
				headers := http.Header{
					"X-Request-ID": []string{"req-123"},
					"X-Version":    []string{"v1.0"},
				}
				return hx.OK("response", hx.WithHeaders(headers))
			},
			method: "GET",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"response"`),
				hasHeaders(http.Header{
					"X-Request-ID": []string{"req-123"},
					"X-Version":    []string{"v1.0"},
				}),
			),
		},
		{
			name: "conflict_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.Conflict("resource already exists")
			},
			method: "POST",
			checks: checks(
				hasStatus(http.StatusConflict),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusConflict,
					Title:      "resource already exists",
					Instance:   "inst-abc123",
				}),
			),
		},
		{
			name: "forbidden_response",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.Forbidden("access denied")
			},
			method: "POST",
			checks: checks(
				hasStatus(http.StatusForbidden),
				hasProblemDetails(hx.ProblemDetails{
					StatusCode: http.StatusForbidden,
					Title:      "access denied",
					Instance:   "inst-abc123",
				}),
			),
		},
		{
			name: "basic_problem_details",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.Problem(
					http.StatusBadRequest,
					"Validation Failed",
					hx.WithDetail("invalid input"),
					hx.WithTypeURI("https://example.com/errors#invalid-input"),
					hx.WithField(hx.F("A", "B")),
				)
			},
			method: "POST",
			checks: checks(
				hasStatus(http.StatusBadRequest),
				hasBody(`{"A":"B","detail":"invalid input","instance":"inst-abc123","status":400,"title":"Validation Failed","type":"https://example.com/errors#invalid-input"}`),
				hasProblemDetails(hx.ProblemDetails{
					Type:       "https://example.com/errors#invalid-input",
					StatusCode: http.StatusBadRequest,
					Title:      "Validation Failed",
					Detail:     "invalid input",
					Instance:   "inst-abc123",
					Extensions: map[string]any{
						"A": "B",
					},
				}),
			),
		},
		{
			name: "with_cookies",
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK(struct {
					Age int
				}{
					Age: 33,
				}, hx.WithCookie(crum.NewCookie("k", "v").SameSiteStrict().MustBuild()))
			},
			method: "POST",
			body:   strings.NewReader(`{"age": "Not a number"}`),
			checks: checks(
				hasStatus(http.StatusOK),
				hasCookie("k", "k=v; Path=/; HttpOnly; Secure; SameSite=Strict"),
			),
		},
		{
			name: "remove_cookie",
			handler: func(ctx context.Context, r *http.Request) error {
				type response struct {
					Age int `json:"age"`
				}

				fakeTime := func() time.Time {
					return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
				}

				return hx.OK(
					response{
						Age: 33,
					},
					hx.WithCookie(
						crum.NewCookie("k", "").
							WithClock(fakeTime).
							Delete().
							MustBuild(),
					),
				)
			},
			method: "POST",
			body:   strings.NewReader(`{"age": "Not a number"}`),
			checks: checks(
				hasStatus(http.StatusOK),
				hasCookie("k", "k=; Path=/; Expires=Wed, 01 Jan 1969 00:00:00 GMT; Max-Age=0; HttpOnly; Secure; SameSite=Lax"),
			),
		},
		{
			name: "bind_error",
			handler: func(ctx context.Context, r *http.Request) error {
				var req struct {
					Age int `json:"age"`
				}
				if err := hx.Bind(r, &req); err != nil {
					return hx.BindProblem(
						err,
						"invalid request",
						hx.WithTypeURI("https://example.com/contact"),
					)
				}

				return hx.OK(struct {
					Age int
				}{
					Age: req.Age,
				})
			},
			method: "POST",
			body:   strings.NewReader(`{"age": "Not a number"}`),
			checks: checks(
				hasStatus(http.StatusBadRequest),
				hasProblemDetails(hx.ProblemDetails{
					Type:       "https://example.com/contact",
					StatusCode: http.StatusBadRequest,
					Title:      "invalid request",
					Instance:   "inst-abc123",
					Extensions: map[string]any{
						"errors": []any{
							map[string]any{
								"field":  "age",
								"detail": "json: cannot unmarshal string into Go value of type int",
							},
						},
					},
				}),
			),
		},
		{
			name: "bind_override_instance",
			handler: func(ctx context.Context, r *http.Request) error {
				var req struct {
					Age int `json:"age"`
				}
				if err := hx.Bind(r, &req); err != nil {
					return hx.BindProblem(
						err,
						"invalid request",
						hx.WithInstance("custom-instance"),
						hx.WithTypeURI("https://example.com/contact"),
					)
				}

				return hx.OK(struct {
					Age int
				}{
					Age: req.Age,
				})
			},
			method: "POST",
			body:   strings.NewReader(`{"age": "Not a number"}`),
			checks: checks(
				hasStatus(http.StatusBadRequest),
				hasProblemDetails(hx.ProblemDetails{
					Type:       "https://example.com/contact",
					StatusCode: http.StatusBadRequest,
					Title:      "invalid request",
					Instance:   "custom-instance",
					Extensions: map[string]any{
						"errors": []any{
							map[string]any{
								"field":  "age",
								"detail": "json: cannot unmarshal string into Go value of type int",
							},
						},
					},
				}),
			),
		},
		{
			name: "bind_custom_fields",
			handler: func(ctx context.Context, r *http.Request) error {
				var req struct {
					Age int `json:"age"`
				}
				if err := hx.Bind(r, &req); err != nil {
					return hx.BindProblem(
						err,
						"invalid request",
						hx.WithTypeURI("https://example.com/contact"),
						hx.WithField(hx.F("age", "Age must be a number")),
					)
				}

				return hx.OK(struct {
					Age int
				}{
					Age: req.Age,
				})
			},
			method: "POST",
			body:   strings.NewReader(`{"age": "Not a number"}`),
			checks: checks(
				hasStatus(http.StatusBadRequest),
				hasProblemDetails(hx.ProblemDetails{
					Type:       "https://example.com/contact",
					StatusCode: http.StatusBadRequest,
					Title:      "invalid request",
					Instance:   "inst-abc123",
					Extensions: map[string]any{
						"errors": []any{
							map[string]any{
								"field":  "age",
								"detail": "json: cannot unmarshal string into Go value of type int",
							},
						},
						"age": "Age must be a number",
					},
				}),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := hx.New(
				hx.WithProblemInstanceGetter(func(ctx context.Context) string {
					return "inst-abc123"
				}),
				hx.WithCustomMux(http.NewServeMux()),
			)
			server.Handle("/"+url.PathEscape(tt.name), tt.handler)

			// Create a test request
			req := httptest.NewRequest(tt.method, "/"+url.PathEscape(tt.name), tt.body)
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Serve the request
			server.ServeHTTP(w, req)
			for _, check := range tt.checks {
				check(t, w)
			}
		})
	}
}

func TestHX_Group(t *testing.T) {
	type check func(t *testing.T, wr *httptest.ResponseRecorder)
	checks := func(ch ...check) []check { return ch }

	hasStatus := func(status int) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			if wr.Code != status {
				t.Errorf("expected status %d, got %d", status, wr.Code)
			}
		}
	}

	hasBody := func(body string) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			if strings.TrimSpace(wr.Body.String()) != body {
				t.Errorf("expected body %q, got %q", body, wr.Body.String())
			}
		}
	}

	tests := []struct {
		name   string
		setup  func(server *hx.HX)
		method string
		path   string
		checks []check
	}{
		{
			name: "prefix_is_applied",
			setup: func(server *hx.HX) {
				api := server.Group("/api/v1")
				api.Handle("/users", func(ctx context.Context, r *http.Request) error {
					return hx.OK("users")
				})
			},
			method: "GET",
			path:   "/api/v1/users",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"users"`),
			),
		},
		{
			name: "method_pattern_is_handled",
			setup: func(server *hx.HX) {
				api := server.Group("/api/v1")
				api.Handle("POST /orders", func(ctx context.Context, r *http.Request) error {
					return hx.Created("order created")
				})
			},
			method: "POST",
			path:   "/api/v1/orders",
			checks: checks(
				hasStatus(http.StatusCreated),
				hasBody(`"order created"`),
			),
		},
		{
			name: "nested_group_compounds_prefix",
			setup: func(server *hx.HX) {
				api := server.Group("/api/v1")
				admin := api.Group("/admin")
				admin.Handle("/stats", func(ctx context.Context, r *http.Request) error {
					return hx.OK("stats")
				})
			},
			method: "GET",
			path:   "/api/v1/admin/stats",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"stats"`),
			),
		},
		{
			name: "method_pattern_in_nested_group",
			setup: func(server *hx.HX) {
				api := server.Group("/api/v1")
				admin := api.Group("/admin")
				admin.Handle("DELETE /users", func(ctx context.Context, r *http.Request) error {
					return hx.OK("deleted")
				})
			},
			method: "DELETE",
			path:   "/api/v1/admin/users",
			checks: checks(
				hasStatus(http.StatusOK),
				hasBody(`"deleted"`),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := hx.New(hx.WithCustomMux(http.NewServeMux()))
			tt.setup(server)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			for _, check := range tt.checks {
				check(t, w)
			}
		})
	}
}

func TestHX_Group_Middleware(t *testing.T) {
	recordingMiddleware := func(name string, calls *[]string) hx.Middleware {
		return func(next hx.HandlerFunc) hx.HandlerFunc {
			return func(ctx context.Context, r *http.Request) error {
				*calls = append(*calls, name)
				return next(ctx, r)
			}
		}
	}

	noop := func(ctx context.Context, r *http.Request) error { return hx.OK("ok") }

	t.Run("group_middleware_is_applied", func(t *testing.T) {
		var calls []string
		server := hx.New(hx.WithCustomMux(http.NewServeMux()))
		api := server.Group("/api", recordingMiddleware("group", &calls))
		api.Handle("/ping", noop)

		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/ping", nil))

		if len(calls) != 1 || calls[0] != "group" {
			t.Errorf("expected [group], got %v", calls)
		}
	})

	t.Run("nested_group_inherits_parent_middleware", func(t *testing.T) {
		var calls []string
		server := hx.New(hx.WithCustomMux(http.NewServeMux()))
		api := server.Group("/api", recordingMiddleware("parent", &calls))
		admin := api.Group("/admin", recordingMiddleware("child", &calls))
		admin.Handle("/stats", noop)

		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/admin/stats", nil))

		if len(calls) != 2 || calls[0] != "parent" || calls[1] != "child" {
			t.Errorf("expected [parent child], got %v", calls)
		}
	})

	t.Run("handler_middleware_stacks_on_group", func(t *testing.T) {
		var calls []string
		server := hx.New(hx.WithCustomMux(http.NewServeMux()))
		api := server.Group("/api", recordingMiddleware("group", &calls))
		api.Handle("/ping", noop, recordingMiddleware("handler", &calls))

		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/ping", nil))

		if len(calls) != 2 || calls[0] != "group" || calls[1] != "handler" {
			t.Errorf("expected [group handler], got %v", calls)
		}
	})

	t.Run("sibling_groups_do_not_share_middleware", func(t *testing.T) {
		var calls []string
		server := hx.New(hx.WithCustomMux(http.NewServeMux()))
		api := server.Group("/api")
		api.Group("/a", recordingMiddleware("mid-a", &calls)).Handle("/ping", noop)
		api.Group("/b").Handle("/ping", noop)

		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/b/ping", nil))

		if len(calls) != 0 {
			t.Errorf("expected no middleware calls for /b, got %v", calls)
		}
	})
}

func TestHX_HijackResponseWriter(t *testing.T) {
	server := hx.New(
		hx.WithProblemInstanceGetter(func(ctx context.Context) string {
			return "inst-abc123"
		}),
		hx.WithCustomMux(http.NewServeMux()),
	)
	server.Handle("/", func(ctx context.Context, r *http.Request) error {
		rw := hx.HijackResponseWriter(ctx)

		rw.Write([]byte("hello"))
		return hx.NoContent()
	})

	// Create a test request
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()

	// Serve the request
	server.ServeHTTP(w, req)

	if w.Body.String() != "hello" {
		t.Errorf("expected body to be 'hello', got %q", w.Body.String())
	}
}
