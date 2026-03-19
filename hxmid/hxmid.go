// Package hxmid provides standard middleware for hx applications.
package hxmid

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/internal"
)

// AttrsFunc extracts additional slog attributes from the request context.
// Use this to attach trace IDs, user IDs, or any other contextual fields to log entries.
type AttrsFunc func(ctx context.Context, r *http.Request) []slog.Attr

// Logger logs each request with its method, path, status code, and duration.
// Requests resulting in 5xx are logged at Error level, 4xx at Warn, everything
// else at Info.
//
// Additional attributes can be injected per-request via optional AttrsFuncs:
//
//	hxmid.Logger(log, func(ctx context.Context, r *http.Request) []slog.Attr {
//	    return []slog.Attr{
//	        slog.String("trace_id", trace.SpanFromContext(ctx).SpanContext().TraceID().String()),
//	    }
//	})
//
// AttrsFunc are called before the request is handled, so they can access the request.
func Logger(log *slog.Logger, extras ...AttrsFunc) hx.Middleware {
	return func(next hx.HandlerFunc) hx.HandlerFunc {
		return func(ctx context.Context, r *http.Request) error {
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
			}

			for _, fn := range extras {
				for _, a := range fn(ctx, r) {
					attrs = append(attrs, a)
				}
			}

			start := time.Now()
			err := next(ctx, r)
			duration := time.Since(start)
			attrs = append(attrs, "duration", duration)

			_, rwRead := internal.PeekResponseWriter(ctx)

			if cause := errors.Unwrap(err); cause != nil {
				attrs = append(attrs, "cause", cause)
			}

			if rwRead.Load() {
				attrs = append(attrs, "hijacked", "true")
			} else {
				attrs = append(attrs, "status", statusFromErr(err))
			}

			switch status := statusFromErr(err); {
			case status >= 500 && !rwRead.Load():
				log.ErrorContext(ctx, "request", attrs...)
			case status >= 400 && !rwRead.Load():
				log.WarnContext(ctx, "request", attrs...)
			default:
				log.InfoContext(ctx, "request", attrs...)
			}

			return err
		}
	}
}

// Recoverer catches panics in downstream handlers, logs them with a stack trace,
// and returns a 500 Internal Server Error response.
func Recoverer(log *slog.Logger) hx.Middleware {
	return func(next hx.HandlerFunc) hx.HandlerFunc {
		return func(ctx context.Context, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(ctx, "hxmid: panic recovered",
						"method", r.Method,
						"path", r.URL.Path,
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					err = hx.Problem(http.StatusInternalServerError, "internal server error")
				}
			}()
			return next(ctx, r)
		}
	}
}

// RequireJSON rejects requests with a body (POST, PUT, PATCH) that do not declare
// Content-Type: application/json, responding with 415 Unsupported Media Type.
// Optional ProblemOpts are forwarded to the error response, e.g. hx.WithTypeURI for docs links.
func RequireJSON(opts ...hx.ProblemOpt) hx.Middleware {
	problemOpts := append([]hx.ProblemOpt{hx.WithDetail("Content-Type must be application/json")}, opts...)
	return func(next hx.HandlerFunc) hx.HandlerFunc {
		return func(ctx context.Context, r *http.Request) error {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if !strings.EqualFold(mt, "application/json") {
					return hx.Problem(http.StatusUnsupportedMediaType, "unsupported media type", problemOpts...)
				}
			}
			return next(ctx, r)
		}
	}
}

// statusFromErr derives the HTTP status code from a HandlerFunc return value.
func statusFromErr(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	var pd hx.ProblemDetails
	if errors.As(err, &pd) {
		return pd.StatusCode
	}
	var resp hx.Response
	if errors.As(err, &resp) {
		return resp.StatusCode
	}
	return http.StatusInternalServerError
}
