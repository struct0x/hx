package hx

import (
	"context"
	"log/slog"
	"net/http"
)

type Opt interface {
	applyHXOpt(cfg *HX)
}

type hxOptFunc func(cfg *HX)

func (f hxOptFunc) applyHXOpt(cfg *HX) {
	f(cfg)
}

// WithLogger sets a custom logger for the HX instance.
// The provided logger will be used for error logging and debugging purposes.
// If not set, slog.Default() will be used.
func WithLogger(log *slog.Logger) Opt {
	return hxOptFunc(func(cfg *HX) {
		cfg.logger = log
	})
}

// WithCustomMux sets a custom multiplexer for the HX instance.
// The provided mux will be used for routing HTTP requests.
// If not set, http.DefaultServeMux will be used.
func WithCustomMux(mux Mux) Opt {
	return hxOptFunc(func(cfg *HX) {
		cfg.mux = mux
	})
}

// WithMiddlewares sets middleware functions for the HX instance.
// These middlewares will be applied to all handlers in the order they are provided.
// Each middleware should implement the Middleware interface.
func WithMiddlewares(m ...Middleware) Opt {
	return hxOptFunc(func(cfg *HX) {
		cfg.mids = m
	})
}

// WithProblemInstanceGetter sets a function that provides the "instance" value
// for ProblemDetails. This is particularly useful in distributed tracing scenarios
// or when using error tracking systems like Sentry, as it allows linking specific
// error instances to their corresponding traces or external error reports.
// The provided function receives a context and should return a string identifier
// that uniquely represents this error occurrence.
func WithProblemInstanceGetter(f func(ctx context.Context) string) Opt {
	return hxOptFunc(func(cfg *HX) {
		cfg.problemInstanceGetter = f
	})
}

// ResponseOpt is an interface for options that can modify both Response and ProblemDetails objects.
// It provides methods to apply modifications to these types, allowing for flexible configuration
// of HTTP responses. Implementations of this interface can modify headers, cookies, and other
// response attributes consistently across both normal responses and problem details.
type ResponseOpt interface {
	// applyResponseOpt applies the option to a Response object.
	applyResponseOpt(*Response)
}

type ProblemOpt interface {
	// applyProblemOpt applies the option to a ProblemDetails object.
	applyProblemOpt(*ProblemDetails)
}

// WithContentType sets the Content-Type header for the response.
// The provided content type string will be used as the value for the Content-Type header.
// This option only affects Response objects and has no effect on ProblemDetails.
// For ProblemDetails, the Content-Type is always set to "application/problem+json".
func WithContentType(ct string) responseOpt {
	return func(r *Response) {
		r.ContentType = ct
	}
}

// WithHeaders sets custom HTTP headers for the response. It accepts an http.Header
// map and returns a ResponseOpt that can be used to modify a Response object.
// The provided headers will be used in the final HTTP response.
func WithHeaders(headers http.Header) sharedOpt {
	return sharedOpt{
		responseOpt: func(r *Response) {
			r.Headers = headers
		},
		problemOpt: func(details *ProblemDetails) {
			details.Headers = headers
		},
	}
}

// WithCookie adds an HTTP cookie to the response. It accepts a pointer to an http.Cookie
// and returns a ResponseOpt that can be used to modify a Response object.
// The provided cookie will be included in the final HTTP response.
func WithCookie(c *http.Cookie) sharedOpt {
	return sharedOpt{
		responseOpt: func(r *Response) {
			if r.Cookies == nil {
				r.Cookies = []*http.Cookie{}
			}
			r.Cookies = append(r.Cookies, c)
		},
		problemOpt: func(details *ProblemDetails) {
			if details.Cookies == nil {
				details.Cookies = []*http.Cookie{}
			}

			details.Cookies = append(details.Cookies, c)
		},
	}
}

// WithHeader sets a single HTTP header for the response. It accepts a key-value pair
// representing the header name and value, and returns a ResponseOpt that can be used
// to modify a Response object. The provided header will be added to the final HTTP response.
func WithHeader(key, value string) sharedOpt {
	return sharedOpt{
		responseOpt: func(r *Response) {
			if r.Headers == nil {
				r.Headers = http.Header{}
			}
			r.Headers.Set(key, value)
		},
		problemOpt: func(details *ProblemDetails) {
			if details.Headers == nil {
				details.Headers = http.Header{}
			}

			details.Headers.Set(key, value)
		},
	}
}

type sharedOpt struct {
	responseOpt func(*Response)
	problemOpt  func(*ProblemDetails)
}

func (r sharedOpt) applyResponseOpt(resp *Response) {
	r.responseOpt(resp)
}

func (r sharedOpt) applyProblemOpt(details *ProblemDetails) {
	r.problemOpt(details)
}

type responseOpt func(*Response)

func (f responseOpt) applyResponseOpt(r *Response) {
	f(r)
}

type problemOpt func(*ProblemDetails)

func (f problemOpt) applyProblemOpt(p *ProblemDetails) {
	f(p)
}
