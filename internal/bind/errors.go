package bind

import (
	"fmt"
	"strings"
)

// Error represents a failure to bind a single struct field.
type Error struct {
	Field string // struct field name
	Err   error  // underlying error (conversion, missing tag, etc.)
}

// Error implements the error interface.
func (e Error) Error() string {
	return fmt.Sprintf("field %q: %v", e.Field, e.Err)
}

// Errors aggregate one or more BindError values.
// It implements the error interface.
type Errors struct {
	Errors []Error
}

func (es *Errors) Append(err Error) {
	es.Errors = append(es.Errors, err)
}

// Error returns a human‑readable summary.
func (es *Errors) Error() string {
	if len(es.Errors) == 0 {
		return ""
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%d binding error(s):\n", len(es.Errors))

	for i, e := range es.Errors {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.Error())

	}
	return b.String()
}

// Unwrap enables errors.Is / errors.As to work on individual errors.
func (es *Errors) Unwrap() []error {
	errs := make([]error, len(es.Errors))
	for i, e := range es.Errors {
		errs[i] = e
	}
	return errs
}
