package internal

import "context"

type errHolderKey struct{}

type ErrHolder struct{ Err error }

// WithErrHolder stores a new ErrHolder in the context and returns both.
func WithErrHolder(ctx context.Context) (context.Context, *ErrHolder) {
	h := &ErrHolder{}
	return context.WithValue(ctx, errHolderKey{}, h), h
}

// ErrorFromContext returns the handler error stored by hx after the handler returned.
// Returns nil if no holder is present.
func ErrorFromContext(ctx context.Context) error {
	if h, ok := ctx.Value(errHolderKey{}).(*ErrHolder); ok {
		return h.Err
	}
	return nil
}
