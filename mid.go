package hx

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/struct0x/hx/internal"
)

// Middleware wraps a HandlerFunc to form a processing chain.
// Return the next handler's error to propagate it or return a new error to short-circuit.
//
// Example:
//
//	func authMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
//	    return func(ctx context.Context, r *http.Request) error {
//	        if r.Header.Get("Authorization") == "" {
//	            return hx.Unauthorized("missing authorization header")
//	        }
//	        return next(ctx, r)
//	    }
//	}
type Middleware func(HandlerFunc) HandlerFunc

// AdaptMiddleware converts standard net/http middleware into a hx Middleware.
//
// It works correctly for middleware that:
//   - Enriches the request (adds context values, sets headers)
//   - Short-circuits by writing a response directly (e.g. rate limiters)
//
// It does not work for middleware that transforms the ResponseWriter itself
// (e.g. gzip compression). Apply those at the server level instead.
func AdaptMiddleware(m func(http.Handler) http.Handler) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, r *http.Request) error {
			var captured error
			nextHTTP := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				captured = next(r.Context(), r)
			})

			w, rwRead := internal.PeekResponseWriter(ctx)
			m(nextHTTP).ServeHTTP(&trackingWriter{ResponseWriter: w, flag: rwRead}, r)
			return captured
		}
	}
}

// chain applies middlewares around handler, first middleware is outermost.
func chain(handler HandlerFunc, mids []Middleware) HandlerFunc {
	for i := len(mids) - 1; i >= 0; i-- {
		handler = mids[i](handler)
	}
	return handler
}

type trackingWriter struct {
	http.ResponseWriter
	flag *atomic.Bool
}

func (t *trackingWriter) WriteHeader(status int) {
	t.flag.Store(true)
	t.ResponseWriter.WriteHeader(status)
}

func (t *trackingWriter) Write(b []byte) (int, error) {
	t.flag.Store(true)
	return t.ResponseWriter.Write(b)
}
