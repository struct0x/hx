package hx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/struct0x/hx"
)

func TestAdaptMiddleware(t *testing.T) {
	type check func(t *testing.T, wr *httptest.ResponseRecorder)
	checks := func(ch ...check) []check { return ch }

	hasStatus := func(status int) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			t.Helper()
			if wr.Code != status {
				t.Errorf("expected status %d, got %d", status, wr.Code)
			}
		}
	}

	hasBody := func(body string) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			t.Helper()
			if wr.Body.String() != body {
				t.Errorf("expected body %q, got %q", body, wr.Body.String())
			}
		}
	}

	hasHeader := func(key, value string) check {
		return func(t *testing.T, wr *httptest.ResponseRecorder) {
			t.Helper()
			if got := wr.Header().Get(key); got != value {
				t.Errorf("header %q: expected %q, got %q", key, value, got)
			}
		}
	}

	tests := []struct {
		name       string
		middleware func(http.Handler) http.Handler
		handler    hx.HandlerFunc
		checks     []check
	}{
		{
			name: "passthrough_propagates_handler_response",
			middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r)
				})
			},
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK("hello")
			},
			checks: checks(
				hasStatus(http.StatusOK),
			),
		},
		{
			name: "passthrough_sets_header_before_next",
			middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("X-From-Middleware", "yes")
					next.ServeHTTP(w, r)
				})
			},
			handler: func(ctx context.Context, r *http.Request) error {
				return hx.OK("hello")
			},
			checks: checks(
				hasStatus(http.StatusOK),
				hasHeader("X-From-Middleware", "yes"),
			),
		},
		{
			name: "shortcircuit_via_write_preserves_response",
			middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte("rate limited"))
					// does not call next
				})
			},
			handler: func(ctx context.Context, r *http.Request) error {
				t.Fatal("handler should not be called when middleware short-circuits")
				return nil
			},
			checks: checks(
				hasStatus(http.StatusTooManyRequests),
				hasBody("rate limited"),
			),
		},
		{
			name: "context_enrichment_is_visible_to_handler",
			middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx := context.WithValue(r.Context(), "key", "injected")
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			},
			handler: func(ctx context.Context, r *http.Request) error {
				val, _ := r.Context().Value("key").(string)
				if val != "injected" {
					t.Errorf("expected context value %q, got %q", "injected", val)
				}
				return hx.OK("ok")
			},
			checks: checks(
				hasStatus(http.StatusOK),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := hx.New(hx.WithCustomMux(http.NewServeMux()))
			server.Handle("/", tt.handler, hx.AdaptMiddleware(tt.middleware))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			for _, check := range tt.checks {
				check(t, w)
			}
		})
	}
}
