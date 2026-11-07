package getter

import (
	"context"
	"errors"
)

// WithContext sets the context of the requests made by the getter.
func WithContext(ctx context.Context) Option {
	return func(opts *getterOptions) {
		opts.ctx = ctx
	}
}

func (o getterOptions) requestContext() context.Context {
	if o.ctx == nil {
		return context.Background()
	}

	return o.ctx
}

// ErrNotFound is matched by errors of Get when the requested content does not exist.
var ErrNotFound = errors.New("not found")

// notFoundError keeps the message of the wrapped error while matching ErrNotFound.
type notFoundError struct {
	error
}

func (e notFoundError) Unwrap() error {
	return e.error
}

func (e notFoundError) Is(target error) bool {
	return target == ErrNotFound
}
