package hx

// Problem creates a ProblemDetails instance with the provided status and summary.
func Problem(status int, summary string, opts ...ProblemOpt) ProblemDetails {
	pr := ProblemDetails{
		StatusCode: status,
		Title:      summary,
	}
	for _, o := range opts {
		o.applyProblemOpt(&pr)
	}
	return pr
}

// WithTypeURI sets the Type field of ProblemDetails.
// The type is a URI reference that identifies the problem type.
func WithTypeURI(s string) problemOpt {
	return func(p *ProblemDetails) {
		p.Type = s
	}
}

// WithDetail sets the Detail field of ProblemDetails.
// The detail contains a human-readable explanation specific to this occurrence of the problem.
func WithDetail(s string) problemOpt {
	return func(p *ProblemDetails) {
		p.Detail = s
	}
}

// Field represents a key-value pair that can be added to ProblemDetails extensions.
type Field struct {
	Key string
	Val any
}

// F is a shorthand constructor for creating Field instances.
func F(k string, v any) Field {
	return Field{
		Key: k,
		Val: v,
	}
}

// WithField adds a single field to the Extensions map of ProblemDetails.
// If Extensions is nil, it initializes a new map.
func WithField(f Field) problemOpt {
	return func(p *ProblemDetails) {
		if p.Extensions == nil {
			p.Extensions = map[string]any{}
		}

		p.Extensions[f.Key] = f.Val
	}
}

// WithFields sets multiple fields at once.
func WithFields(kv ...Field) problemOpt {
	return func(p *ProblemDetails) {
		if p.Extensions == nil {
			p.Extensions = map[string]any{}
		}

		for _, f := range kv {
			p.Extensions[f.Key] = f.Val
		}
	}
}

// WithInstance sets the Instance field of ProblemDetails.
// The instance is a URI reference that identifies the specific occurrence of the problem.
func WithInstance(s string) problemOpt {
	return func(p *ProblemDetails) {
		p.Instance = s
	}
}

// WithCause sets the underlying error that caused this problem.
// This error will be logged but not included in the JSON response.
func WithCause(err error) problemOpt {
	return func(p *ProblemDetails) {
		p.Cause = err
	}
}
