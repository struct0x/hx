package internal

import (
	"context"
	"net/http"
	"sync/atomic"
)

type ctxKey string

const rwKey ctxKey = "rw"

// WithResponseWriter stores the http.ResponseWriter in the context.
// It returns a new context containing the response writer.
func WithResponseWriter(ctx context.Context, rwRead *atomic.Bool, rw http.ResponseWriter) context.Context {
	return context.WithValue(ctx, rwKey, func() http.ResponseWriter {
		rwRead.Store(true)
		return rw
	})
}

// HijackResponseWriter retrieves the http.ResponseWriter from the context.
// When the ResponseWriter is hijacked, the return value from HandlerFunc will be ignored.
func HijackResponseWriter(ctx context.Context) http.ResponseWriter {
	rw, ok := ctx.Value(rwKey).(func() http.ResponseWriter)
	if !ok {
		panic("unreachable: no response writer in context")
	}
	return rw()
}
