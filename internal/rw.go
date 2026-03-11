package internal

import (
	"context"
	"net/http"
	"sync/atomic"
)

type ctxKey string

const rwKey ctxKey = "rw"

type rwValue struct {
	rw   http.ResponseWriter
	flag *atomic.Bool
}

// WithResponseWriter stores the http.ResponseWriter in the context.
// It returns a new context containing the response writer.
func WithResponseWriter(ctx context.Context, rwRead *atomic.Bool, rw http.ResponseWriter) context.Context {
	return context.WithValue(ctx, rwKey, &rwValue{rw: rw, flag: rwRead})
}

// HijackResponseWriter retrieves the http.ResponseWriter from the context and marks it as taken.
// When hijacked, the return value from HandlerFunc will be ignored.
func HijackResponseWriter(ctx context.Context) http.ResponseWriter {
	v, ok := ctx.Value(rwKey).(*rwValue)
	if !ok {
		panic("unreachable: no response writer in context")
	}
	v.flag.Store(true)
	return v.rw
}

// PeekResponseWriter retrieves the http.ResponseWriter and its write flag from context
// without marking it as hijacked.
func PeekResponseWriter(ctx context.Context) (http.ResponseWriter, *atomic.Bool) {
	v, ok := ctx.Value(rwKey).(*rwValue)
	if !ok {
		panic("unreachable: no response writer in context")
	}
	return v.rw, v.flag
}
