package capability

import (
	"context"
	"errors"
)

const DispatchAuthorityRevokedCode = "PROJECT_DISPATCH_AUTHORITY_REVOKED"

var ErrDispatchAuthorityRevoked = errors.New("project action authority ended before dispatch")

type dispatchCheckKey struct{}

// WithDispatchCheck attaches a live authorization check to one resolved action.
// Adapters recheck it after waiting for their transport's write lock.
func WithDispatchCheck(ctx context.Context, check func(context.Context) error) context.Context {
	return context.WithValue(ctx, dispatchCheckKey{}, check)
}

func CheckDispatch(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if check, ok := ctx.Value(dispatchCheckKey{}).(func(context.Context) error); ok && check != nil {
		if err := check(ctx); err != nil && ctx.Err() == nil {
			return ErrDispatchAuthorityRevoked
		}
	}
	return ctx.Err()
}
