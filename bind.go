package hx

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/struct0x/hx/internal/bind"
)

type BindOpt = bind.Opt

// WithPathValueFunc overrides the default way of extracting a path
// parameter from the request.  The function receives the request and the
// name of the path variable and must return the value (or the empty string
// if the variable is not present).
func WithPathValueFunc(fn func(r *http.Request, name string) string) BindOpt {
	return bind.WithPathValueFunc(fn)
}

// WithMaxFormMemoryMB configures the maximum size of multipart form data that will reside in memory.
// The rest of the data will be stored on disk in temporary files.
// This option is used when binding multipart form data and file uploads.
func WithMaxFormMemoryMB(maxFormMemoryMB int64) BindOpt {
	return bind.WithMaxFormMemoryMB(maxFormMemoryMB)
}

// WithValidator configures a custom validator.Validate instance to be used for request validation.
// The validator will be used to validate struct fields with "validate" tags after binding.
// If not provided, a default validator will be used.
func WithValidator(v *validator.Validate) BindOpt {
	return bind.WithValidator(v)
}

// Bind extracts data from an HTTP request into a destination struct.
// It supports binding from multiple sources including URL query parameters, path variables,
// headers, cookies, JSON body, and multipart file uploads.
//
// The destination must be a pointer to a struct. Fields in the struct are bound based on
// struct tags that specify the data source and field name:
//
//   - `query:"name"` - binds from URL query parameters
//   - `path:"name"` - binds from URL path variables
//   - `header:"Name"` - binds from HTTP headers
//   - `cookie:"name"` - binds from HTTP cookies
//   - `json:"name"` - binds from JSON request body (application/json)
//   - `file:"name"` - binds file uploads from multipart/form-data
//
// Supported field types include:
//   - Basic types: string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64
//   - Slices of basic types (for multiple values)
//   - Slices of strings (for headers and query parameters with multiple values)
//   - http.Cookie (for cookie fields)
//   - multipart.FileHeader (for single file uploads)
//   - []*multipart.FileHeader (for multiple file uploads)
//   - Types implementing encoding.TextUnmarshaler
//   - Any type for json-tagged fields (unmarshaled via encoding/json)
//
// Options:
//   - WithPathValueFunc: provides a custom function to extract path variables from the request.
//     By default, uses http.Request.PathValue.
//   - WithMaxFormMemoryMB: sets the maximum memory in megabytes for parsing multipart forms.
//     Defaults to 32 MB if not specified.
//   - WithValidator: sets a custom validator.Validate instance to be used for request validation.
//     By default, a default validator is used.
//
// Returns:
//   - nil if all fields are successfully bound
//   - error if any field fails to bind, use BindProblem to create a structured error response
//
// Example usage:
//
//	type UserRequest struct {
//	    ID       int      `path:"id"`
//	    Name     string   `json:"name"`
//	    Email    string   `json:"email"`
//	    Tags     []string `query:"tags"`
//	    AuthToken string  `header:"Authorization"`
//	}
//
//	func handler(ctx context.Context, r *http.Request) error {
//	    var req UserRequest
//	    if err := hx.Bind(r, &req); err != nil {
//	        return BindProblem(err, "Invalid user request",
//	        	WithTypeURI("https://example.com/errors/invalid-user"))
//	    }
//	    // Use req...
//	    return nil
//	}
func Bind[T any](r *http.Request, dst *T, opts ...BindOpt) error {
	return bind.Bind(r, dst, opts...)
}

// BindProblem creates a structured error response by wrapping binding errors into a ProblemDetails.
// It takes an error from Bind, a summary message, and optional ProblemOpt options.
//
// The function handles different types of binding errors:
//   - Structural errors (nil request, nil destination, invalid types) return 500 Internal Server Error
//   - Validation errors return 400 Bad Request with detailed field errors in the extensions
//   - Other errors return 500 Internal Server Error
//
// For validation errors, the response includes an "errors" field in the extensions containing
// an array of objects with "field" and "detail" properties for each validation error.
//
// Allowed ProblemOpt options:
// - WithTypeURI sets the Type field of ProblemDetails.
// - WithDetail sets the Detail field of ProblemDetails.
// - WithField adds a single field to the Extensions map of ProblemDetails.
// - WithFields sets multiple fields at once.
// - WithInstance sets the Instance field of ProblemDetails.
// Note: WithCause option is automatically added and will be ignored if provided manually.
//
// Example usage:
//
//	var req UserRequest
//	if err := Bind(r, &req); err != nil {
//	    return BindProblem(err, "Invalid user request",
//	        WithTypeURI("https://example.com/errors/invalid-user"))
//	}
func BindProblem(err error, summary string, opts ...ProblemOpt) error {
	pOpts := make([]ProblemOpt, len(opts), len(opts)+2)
	copy(pOpts, opts)

	pOpts = append(pOpts,
		WithCause(err),
	)

	switch {
	case errors.Is(err, bind.ErrNilRequest),
		errors.Is(err, bind.ErrNilDestination),
		errors.Is(err, bind.ErrNotAStruct),
		errors.Is(err, bind.ErrExpectedStruct),
		errors.Is(err, bind.ErrMultipleTags),
		errors.Is(err, bind.ErrEmptyTag):
		return Problem(
			http.StatusInternalServerError,
			summary,
			pOpts...,
		)
	}

	var bindErrs *bind.Errors
	if errors.As(err, &bindErrs) {
		fields := make([]struct {
			Field  string `json:"field"`
			Detail string `json:"detail"`
		}, 0, len(bindErrs.Errors))
		for _, e := range bindErrs.Errors {
			detail := e.Err.Error()

			var fieldErr validator.FieldError
			if errors.As(e.Err, &fieldErr) {
				detail = fieldErr.ActualTag()
			}

			fields = append(fields, struct {
				Field  string `json:"field"`
				Detail string `json:"detail"`
			}{
				Field:  e.Field,
				Detail: detail,
			})
		}

		pOpts = append(pOpts,
			WithField(F("errors", fields)),
		)

		return Problem(
			http.StatusBadRequest,
			summary,
			pOpts...,
		)
	}

	return Problem(
		http.StatusInternalServerError,
		summary,
		pOpts...,
	)
}
