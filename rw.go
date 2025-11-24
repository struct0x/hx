package hx

import (
	"context"
	"net/http"

	"github.com/struct0x/hx/internal"
)

// HijackResponseWriter retrieves the http.ResponseWriter from the context.
// When the ResponseWriter is hijacked, the return value from HandlerFunc will be ignored.
func HijackResponseWriter(ctx context.Context) http.ResponseWriter {
	return internal.HijackResponseWriter(ctx)
}
