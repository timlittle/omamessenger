package server

import (
	"context"
	"encoding/json"
	"fmt"
)

// method decodes a request's params and runs it.
type method func(ctx context.Context, params json.RawMessage) (any, error)

// bind adapts a handler taking typed params to a method.
func bind[P any](handle func(ctx context.Context, p P) (any, error)) method {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p P
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("%w: %w", errBadParams, err)
		}

		return handle(ctx, p)
	}
}
