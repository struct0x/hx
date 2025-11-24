package hx

import (
	"net/http"

	"github.com/struct0x/hx/internal/out"
)

// OK creates a successful HTTP response with a 200 (OK) status code.
// It accepts a body of any type and optional response modifiers.
func OK(body any, opts ...ResponseOpt) *Response {
	return Respond(http.StatusOK, body, opts...)
}

// Created creates an HTTP response with a 201 (Created) status code.
// It accepts a body of any type and optional response modifiers.
func Created(body any, opts ...ResponseOpt) *Response {
	return Respond(http.StatusCreated, body, opts...)
}

// BadRequest creates an HTTP response with a 400 (Bad Request) status code.
// It accepts a body of any type and optional response modifiers.
func BadRequest(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusBadRequest, title, opts...)
}

// NotAllowed creates an HTTP response with a 405 (Method Not Allowed) status code.
// It accepts a title string describing the error and optional response modifiers.
// Returns a ProblemDetails object that represents the error response.
func NotAllowed(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusMethodNotAllowed, title, opts...)
}

// Unauthorized creates an HTTP response with a 401 (Unauthorized) status code.
// It accepts a body of any type and optional response modifiers.
func Unauthorized(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusUnauthorized, title, opts...)
}

// Forbidden creates an HTTP response with a 403 (Forbidden) status code.
// It accepts a body of any type and optional response modifiers.
func Forbidden(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusForbidden, title, opts...)
}

// NotFound creates an HTTP response with a 404 (Not Found) status code.
// It accepts a body of any type and optional response modifiers.
func NotFound(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusNotFound, title, opts...)
}

// MethodNotAllowed creates an HTTP response with a 405 (Method Not Allowed) status code.
// It accepts a body of any type and optional response modifiers.
func MethodNotAllowed(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusMethodNotAllowed, title, opts...)
}

// Conflict creates an HTTP response with a 409 (Conflict) status code.
// It accepts a body of any type and optional response modifiers.
func Conflict(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusConflict, title, opts...)
}

// Internal creates an HTTP response with a 500 (Internal Server Error) status code.
func Internal(title string, opts ...ProblemOpt) ProblemDetails {
	return Problem(http.StatusInternalServerError, title, opts...)
}

// Respond creates an HTTP response with the specified status code and body.
// It accepts a status code, a body of any type, and optional response modifiers.
func Respond(status int, body any, opts ...ResponseOpt) *Response {
	r := out.Response{
		ContentType: "application/json",
		StatusCode:  status,
		Body:        body,
	}

	for _, o := range opts {
		o.applyResponseOpt(&r)
	}

	return &r
}
