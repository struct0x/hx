package hx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/struct0x/hx/internal"
	"github.com/struct0x/hx/internal/out"
)

// ProblemDetails is a JSON object that describes an error.
// https://datatracker.ietf.org/doc/html/rfc9457
type ProblemDetails = out.ProblemDetails

// Response representing an HTTP response with status, body, and headers.
type Response = out.Response

// HandlerFunc is a function type that handles HTTP requests in HX framework.
// It receives a context.Context and *http.Request as input parameters and returns an error.
// Context is identical to http.Request.Context, but it includes a ResponseWriter that can be hijacked.
//
// If HandlerFunc returns:
// - nil: panics in dev mode, 500 in production — use hx.NoContent() for 204 responses
// - ProblemDetails: the response will be encoded as application/problem+json
// - Response: the response will be encoded as application/json with custom headers
// - any other error: the response will be 500 Internal Server Error
//
// Example usage:
//
//	hx.HandlerFunc(func(ctx context.Context, r *http.Request) error {
//	    // Handle the request
//	    return nil // or return an error
//	})
type HandlerFunc func(ctx context.Context, r *http.Request) error

// Mux is an interface that wraps the http.Handler.
type Mux interface {
	http.Handler

	Handle(pattern string, handler http.Handler)
}

// HX is a framework for building HTTP APIs with enhanced error handling and middleware support.
// It provides a convenient way to handle HTTP requests, manage middleware chains, and standardize
// error responses using ProblemDetails (RFC 9457).
//
// Example usage:
//
//	hx := hx.New(
//	    hx.WithLogger(slog.Default()),
//	    hx.WithCustomMux(http.NewServeMux()),
//	    hx.WithMiddleware(loggingMiddleware),
//	)
//
//	// Handle requests
//	hx.Handle("/api/users", func(ctx context.Context, r *http.Request) error {
//	    // Handle the request
//	    return nil
//	})
//
//	// Start the server
//	http.ListenAndServe(":8080", hx)
type HX struct {
	logger     *slog.Logger
	mux        Mux
	mids       []Middleware
	prefix     string
	production bool

	problemInstanceGetter func(ctx context.Context) string
}

// New creates a new HX instance.
func New(opts ...Opt) *HX {
	hx := &HX{
		logger: slog.Default(),
		mux:    http.DefaultServeMux,
	}

	for _, o := range opts {
		o.applyHXOpt(hx)
	}

	return hx
}

func (h *HX) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// Handle registers a new request handler with the given pattern and middleware.
func (h *HX) Handle(pattern string, handler HandlerFunc, mids ...Middleware) {
	all := make([]Middleware, len(h.mids), len(h.mids)+len(mids))
	copy(all, h.mids)
	all = append(all, mids...)
	h.mux.Handle(buildPattern(h.prefix, pattern), h.handle(chain(handler, all)))
}

// Group creates a sub-router sharing the same mux, with the given path prefix
// and additional middlewares appended to the current chain.
//
// Example:
//
//	api := server.Group("/api/v1", authMiddleware)
//	api.Handle("POST /users", createUserHandler)   // registers "POST /api/v1/users"
//	api.Handle("/orders", listOrdersHandler)        // registers "/api/v1/orders"
//
//	admin := api.Group("/admin", adminOnlyMiddleware)
//	admin.Handle("/stats", statsHandler)            // registers "/api/v1/admin/stats"
func (h *HX) Group(prefix string, mids ...Middleware) *HX {
	inherited := make([]Middleware, len(h.mids), len(h.mids)+len(mids))
	copy(inherited, h.mids)
	return &HX{
		logger:                h.logger,
		mux:                   h.mux,
		prefix:                h.prefix + prefix,
		mids:                  append(inherited, mids...),
		production:            h.production,
		problemInstanceGetter: h.problemInstanceGetter,
	}
}

// buildPattern prepends prefix to a ServeMux pattern, handling the optional
// method prefix in Go 1.22+ patterns (e.g. "POST /path").
func buildPattern(prefix, pattern string) string {
	if i := strings.Index(pattern, " "); i != -1 {
		return pattern[:i+1] + prefix + pattern[i+1:]
	}
	return prefix + pattern
}

func (h *HX) handle(handler HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rwRead := new(atomic.Bool)

		ctx := internal.WithResponseWriter(r.Context(), rwRead, w)
		hxErr := handler(ctx, r.WithContext(ctx))

		if rwRead.Load() {
			if !h.production && hxErr != nil {
				panic(fmt.Sprintf("hx: hijacked response writer, but handler returned error: %v", hxErr))
			}

			// Handler took control of the response writer.
			// No need to write anything.
			return
		}

		if hxErr == nil {
			if !h.production {
				panic("hx: handler returned nil; use hx.NoContent() for 204 responses")
			}

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		enc := json.NewEncoder(w)

		var pd ProblemDetails
		if errors.As(hxErr, &pd) {
			if h.problemInstanceGetter != nil && pd.Instance == "" {
				pd.Instance = h.problemInstanceGetter(ctx)
			}

			for _, c := range pd.Cookies {
				http.SetCookie(w, c)
			}

			w.Header().Add("Content-Type", "application/problem+json")
			for name, values := range pd.Headers {
				for _, value := range values {
					w.Header().Add(name, value)
				}
			}
			w.WriteHeader(pd.StatusCode)

			if err := enc.Encode(pd); err != nil {
				h.logger.ErrorContext(
					ctx,
					"hx: error encoding problem details",
					"error",
					err,
					"path",
					r.URL.Path,
				)
			}

			return
		}

		var resp out.Response
		if errors.As(hxErr, &resp) {
			for _, c := range resp.Cookies {
				http.SetCookie(w, c)
			}

			w.Header().Add("Content-Type", resp.ContentType)
			for name, values := range resp.Headers {
				for _, value := range values {
					w.Header().Add(name, value)
				}
			}
			w.WriteHeader(resp.StatusCode)

			switch body := resp.Body.(type) {
			case io.Reader:
				_, err := io.Copy(w, body)
				if err != nil {
					h.logger.ErrorContext(ctx, "hx: error copying response body", "error", err)
					return
				}
				return
			}

			if resp.Body != nil {
				if err := enc.Encode(resp.Body); err != nil {
					h.logger.ErrorContext(ctx, "hx: error encoding response body", "error", err)
				}
			}

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		var opts []ProblemOpt
		if h.problemInstanceGetter != nil {
			opts = append(opts, WithInstance(h.problemInstanceGetter(ctx)))
		}

		if err := enc.Encode(Problem(http.StatusInternalServerError, "internal server error", opts...)); err != nil {
			h.logger.ErrorContext(ctx, "hx: error encoding problem details", "error", err)
		}
	})
}
