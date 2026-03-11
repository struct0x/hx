package hxmid_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/hxmid"
)

func newServer(t *testing.T, handler hx.HandlerFunc, mids ...hx.Middleware) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := hx.New(
		hx.WithCustomMux(mux),
		hx.WithProductionMode(true),
	)
	server.Handle("/", handler, mids...)
	return httptest.NewServer(mux)
}

func TestLogger(t *testing.T) {
	tests := []struct {
		name       string
		handler    hx.HandlerFunc
		wantLevel  slog.Level
		wantStatus int
	}{
		{
			name:       "info_on_success",
			handler:    func(ctx context.Context, r *http.Request) error { return hx.OK("ok") },
			wantLevel:  slog.LevelInfo,
			wantStatus: http.StatusOK,
		},
		{
			name:       "warn_on_4xx",
			handler:    func(ctx context.Context, r *http.Request) error { return hx.NotFound("not found") },
			wantLevel:  slog.LevelWarn,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "error_on_5xx",
			handler:    func(ctx context.Context, r *http.Request) error { return hx.Internal("oops") },
			wantLevel:  slog.LevelError,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLevel slog.Level
			var gotStatus int

			handler := slog.NewJSONHandler(nil, nil)
			_ = handler

			// Use a capturing handler to assert log level.
			h := &capturingHandler{fn: func(r slog.Record) {
				gotLevel = r.Level
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "status" {
						gotStatus = int(a.Value.Int64())
					}
					return true
				})
			}}

			srv := newServer(t, tt.handler, hxmid.Logger(slog.New(h)))
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			if gotLevel != tt.wantLevel {
				t.Errorf("log level: got %v, want %v", gotLevel, tt.wantLevel)
			}
			if gotStatus != tt.wantStatus {
				t.Errorf("logged status: got %d, want %d", gotStatus, tt.wantStatus)
			}
		})
	}
}

func TestLogger_ExtraAttrs(t *testing.T) {
	var gotAttrs []slog.Attr

	h := &capturingHandler{fn: func(r slog.Record) {
		r.Attrs(func(a slog.Attr) bool {
			gotAttrs = append(gotAttrs, a)
			return true
		})
	}}

	srv := newServer(t,
		func(ctx context.Context, r *http.Request) error { return hx.OK("ok") },
		hxmid.Logger(slog.New(h),
			func(ctx context.Context, r *http.Request) []slog.Attr {
				return []slog.Attr{slog.String("trace_id", "abc-123")}
			},
			func(ctx context.Context, r *http.Request) []slog.Attr {
				return []slog.Attr{slog.String("user_id", r.Header.Get("X-User-ID"))}
			},
		),
	)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.Header.Set("X-User-ID", "user-42")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	find := func(key string) (string, bool) {
		for _, a := range gotAttrs {
			if a.Key == key {
				return a.Value.String(), true
			}
		}
		return "", false
	}

	if v, ok := find("trace_id"); !ok || v != "abc-123" {
		t.Errorf("trace_id: got %q, want %q", v, "abc-123")
	}
	if v, ok := find("user_id"); !ok || v != "user-42" {
		t.Errorf("user_id: got %q, want %q", v, "user-42")
	}
}

func TestRecoverer(t *testing.T) {
	t.Run("recovers_panic_and_returns_500", func(t *testing.T) {
		srv := newServer(t,
			func(ctx context.Context, r *http.Request) error {
				panic("something exploded")
			},
			hxmid.Recoverer(slog.Default()),
		)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", resp.StatusCode)
		}
	})

	t.Run("does_not_interfere_with_normal_handler", func(t *testing.T) {
		srv := newServer(t,
			func(ctx context.Context, r *http.Request) error { return hx.OK("ok") },
			hxmid.Recoverer(slog.Default()),
		)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestRequireJSON(t *testing.T) {
	noop := func(ctx context.Context, r *http.Request) error { return hx.OK("ok") }

	tests := []struct {
		name        string
		method      string
		contentType string
		wantStatus  int
	}{
		{
			name:        "post_with_json_passes",
			method:      http.MethodPost,
			contentType: "application/json",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "post_with_json_charset_passes",
			method:      http.MethodPost,
			contentType: "application/json; charset=utf-8",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "post_without_content_type_rejected",
			method:      http.MethodPost,
			contentType: "",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "post_with_form_rejected",
			method:      http.MethodPost,
			contentType: "application/x-www-form-urlencoded",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "put_without_json_rejected",
			method:      http.MethodPut,
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "patch_without_json_rejected",
			method:      http.MethodPatch,
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "get_skips_check",
			method:      http.MethodGet,
			contentType: "",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "delete_skips_check",
			method:      http.MethodDelete,
			contentType: "",
			wantStatus:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, noop, hxmid.RequireJSON())
			defer srv.Close()

			req, _ := http.NewRequest(tt.method, srv.URL+"/", strings.NewReader("{}"))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

// capturingHandler is a slog.Handler that calls fn for each record.
type capturingHandler struct {
	fn func(slog.Record)
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler              { return h }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.fn(r)
	return nil
}
