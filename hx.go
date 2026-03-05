package hx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/justinas/alice"

	"github.com/struct0x/hx/internal"
	"github.com/struct0x/hx/internal/out"
)

// ErrorFromContext returns the handler error after the handler has returned.
// Call this from middleware after invoking next.ServeHTTP.
func ErrorFromContext(ctx context.Context) error {
	return internal.ErrorFromContext(ctx)
}

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
// - nil: the response will be 204 No Content
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

// Middleware is a function that wraps a HandlerFunc.
type Middleware = alice.Constructor

// HX is a framework for building HTTP APIs with enhanced error handling and middleware support.
// It provides a convenient way to handle HTTP requests, manage middleware chains, and standardize
// error responses using ProblemDetails (RFC 9457).
//
// Example usage:
//
//	hx := hx.New(
//	    hx.WithLogger(slog.Default()),
//	    hx.WithMux(http.NewServeMux()),
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
	logger *slog.Logger
	mux    Mux
	mids   []Middleware

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
	mid := alice.New(h.mids...).Append(mids...)
	h.mux.Handle(pattern, mid.Then(h.handle(handler)))
}

func (h *HX) handle(handler HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rwRead := new(atomic.Bool)

		ctx := internal.WithResponseWriter(r.Context(), rwRead, w)
		ctx, holder := internal.WithErrHolder(ctx)

		*r = *r.WithContext(ctx)
		hxErr := handler(ctx, r)

		if rwRead.Load() {
			// Handler took control of the response writer.
			// No need to write anything.
			return
		}

		if hxErr == nil {
			// Nil errors mean 204 No Content
			w.WriteHeader(http.StatusNoContent)
			return
		}

		enc := json.NewEncoder(w)

		var pd ProblemDetails
		if errors.As(hxErr, &pd) {
			holder.Err = hxErr

			if pd.Cause != nil {
				h.logger.Error("hx: request failed",
					"method", r.Method,
					"path", r.URL.Path,
					"error", pd.Cause)
			}

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

		var resp *out.Response
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

			switch resp.Body.(type) {
			case io.Reader:
				_, err := io.Copy(w, resp.Body.(io.Reader))
				if err != nil {
					h.logger.ErrorContext(ctx, "hx: error copying response body", "error", err)
					return
				}
				return
			}

			if err := enc.Encode(resp.Body); err != nil {
				h.logger.ErrorContext(ctx, "hx: error encoding response body", "error", err)
			}

			return
		}

		holder.Err = hxErr
		h.logger.ErrorContext(ctx, "hx: request with unknown error",
			"method", r.Method,
			"path", r.URL.Path,
			"error", hxErr)

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
