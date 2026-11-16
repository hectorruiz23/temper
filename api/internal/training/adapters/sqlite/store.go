package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/hectorruiz23/temper/api/internal/training/app"
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

type transaction struct {
	query queryer
}

func (s *Store) WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx app.Transaction) error) error {
	databaseTransaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer databaseTransaction.Rollback()

	if err := fn(ctx, &transaction{query: databaseTransaction}); err != nil {
		return err
	}
	return databaseTransaction.Commit()
}

func (t *transaction) FindReceipt(ctx context.Context, operation, key string) (app.Receipt, bool, error) {
	var receipt app.Receipt
	err := t.query.QueryRowContext(ctx, `SELECT request_hash, result_json FROM idempotency_request WHERE operation = ? AND key = ?`, operation, key).
		Scan(&receipt.RequestHash, &receipt.Result)
	if errors.Is(err, sql.ErrNoRows) {
		return app.Receipt{}, false, nil
	}
	if err != nil {
		return app.Receipt{}, false, err
	}
	return receipt, true, nil
}

func (t *transaction) SaveReceipt(ctx context.Context, operation, key, requestHash, result string, createdAt int64) error {
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	_, err := t.query.ExecContext(ctx, `INSERT INTO idempotency_request (operation, key, request_hash, result_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		operation, key, requestHash, result, createdAt)
	return err
}
