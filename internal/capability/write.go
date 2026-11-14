package capability

import (
	"context"
	"database/sql"
)

type writeAuthorizationKey struct{}

// WithWriteAuthorization attaches a guard for the same database transaction
// that publishes the requested mutation.
func WithWriteAuthorization(ctx context.Context, check func(context.Context, *sql.Tx) error) context.Context {
	return context.WithValue(ctx, writeAuthorizationKey{}, check)
}

func CheckWriteAuthorization(ctx context.Context, tx *sql.Tx) error {
	if check, _ := ctx.Value(writeAuthorizationKey{}).(func(context.Context, *sql.Tx) error); check != nil {
		return check(ctx, tx)
	}
	return nil
}

// ExecAuthorized keeps single-statement writes atomic with a configured guard.
func ExecAuthorized(ctx context.Context, db *sql.DB, query string, args ...any) (sql.Result, error) {
	if check, _ := ctx.Value(writeAuthorizationKey{}).(func(context.Context, *sql.Tx) error); check == nil {
		return db.ExecContext(ctx, query, args...)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err := CheckWriteAuthorization(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
