package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func (s *Store) Execute(ctx context.Context, operation, key, requestHash string, mutate func(context.Context) ([]byte, error)) ([]byte, error) {
	var result []byte

	err := s.run(ctx, func(ctx context.Context, query queryer) error {
		var storedHash string
		var storedResult string
		err := query.QueryRowContext(ctx, `SELECT request_hash, result_json FROM idempotency_request WHERE operation = ? AND key = ?`, operation, key).Scan(&storedHash, &storedResult)
		switch {
		case err == nil:
			if storedHash != requestHash {
				return kernel.NewRuleError("idempotency_conflict", fmt.Sprintf("idempotency key %q was already used with a different request", key))
			}
			result = []byte(storedResult)
			return nil
		case errors.Is(err, sql.ErrNoRows):
		default:
			return err
		}

		payload, err := mutate(ctx)
		if err != nil {
			return err
		}

		if _, err := query.ExecContext(ctx, `INSERT INTO idempotency_request (operation, key, request_hash, result_json, created_at) VALUES (?, ?, ?, ?, ?)`,
			operation, key, requestHash, string(payload), time.Now().UTC().UnixMilli()); err != nil {
			return err
		}

		result = payload
		return nil
	})

	return result, err
}
