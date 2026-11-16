package sqlite

import (
	"context"
	"database/sql"
)

type Store struct {
	database *sql.DB
}

func New(database *sql.DB) *Store {
	return &Store{database: database}
}

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type transactionKey struct{}

func transactionFrom(ctx context.Context) *sql.Tx {
	transaction, _ := ctx.Value(transactionKey{}).(*sql.Tx)
	return transaction
}

func (s *Store) executor(ctx context.Context) queryer {
	if transaction := transactionFrom(ctx); transaction != nil {
		return transaction
	}
	return s.database
}

func (s *Store) run(ctx context.Context, operation func(context.Context, queryer) error) error {
	if transaction := transactionFrom(ctx); transaction != nil {
		return operation(ctx, transaction)
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	ctx = context.WithValue(ctx, transactionKey{}, transaction)
	if err := operation(ctx, transaction); err != nil {
		return err
	}
	return transaction.Commit()
}
