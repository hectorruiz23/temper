package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hectorruiz23/temper/api/internal/health/app"
	"github.com/hectorruiz23/temper/api/internal/health/domain"
)

type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	database *sql.DB
}

type transaction struct {
	database executor
}

func New(database *sql.DB) *Store {
	return &Store{database: database}
}

func (s *Store) WithinTransaction(ctx context.Context, fn func(ctx context.Context, days app.DayStore, idempotency app.IdempotencyStore) error) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	repo := &transaction{database: tx}
	if err := fn(ctx, repo, repo); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetDay(ctx context.Context, date string) (domain.Day, error) {
	return getDay(ctx, s.database, date)
}

func (s *Store) SaveDay(ctx context.Context, day domain.Day) error {
	return saveDay(ctx, s.database, day)
}

func (s *Store) Find(ctx context.Context, operation, key string) (app.Receipt, bool, error) {
	return find(ctx, s.database, operation, key)
}

func (s *Store) Save(ctx context.Context, operation, key, requestHash, result string, createdAt int64) error {
	return saveReceipt(ctx, s.database, operation, key, requestHash, result, createdAt)
}

func (t *transaction) GetDay(ctx context.Context, date string) (domain.Day, error) {
	return getDay(ctx, t.database, date)
}

func (t *transaction) SaveDay(ctx context.Context, day domain.Day) error {
	return saveDay(ctx, t.database, day)
}

func (t *transaction) Find(ctx context.Context, operation, key string) (app.Receipt, bool, error) {
	return find(ctx, t.database, operation, key)
}

func (t *transaction) Save(ctx context.Context, operation, key, requestHash, result string, createdAt int64) error {
	return saveReceipt(ctx, t.database, operation, key, requestHash, result, createdAt)
}

func getDay(ctx context.Context, database executor, date string) (domain.Day, error) {
	var day domain.Day
	var sleepMS sql.NullInt64
	var steps sql.NullInt64
	var restingHRBPM sql.NullInt64
	var hrvRMSSDMS sql.NullFloat64

	err := database.QueryRowContext(ctx, `
		SELECT date, timezone, revision, sleep_ms, steps, resting_hr_bpm, hrv_rmssd_ms, updated_at, last_source
		FROM day
		WHERE date = ?`, date).Scan(
		&day.Date,
		&day.Timezone,
		&day.Revision,
		&sleepMS,
		&steps,
		&restingHRBPM,
		&hrvRMSSDMS,
		&day.UpdatedAt,
		&day.LastSource,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Day{}, domain.ErrDayNotFound
	}
	if err != nil {
		return domain.Day{}, err
	}

	day.SleepMS = nullableInt64(sleepMS)
	day.Steps = nullableInt64(steps)
	day.RestingHRBPM = nullableInt64(restingHRBPM)
	day.HRVRMSSDMS = nullableFloat64(hrvRMSSDMS)

	stages, err := getStages(ctx, database, date)
	if err != nil {
		return domain.Day{}, err
	}
	day.SleepStages = stages

	workouts, err := getWorkouts(ctx, database, date)
	if err != nil {
		return domain.Day{}, err
	}
	day.Workouts = workouts

	return day, nil
}

func getStages(ctx context.Context, database executor, date string) ([]domain.SleepStage, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT stage, duration_ms
		FROM day_sleep_stage
		WHERE day_date = ?
		ORDER BY stage`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stages := make([]domain.SleepStage, 0)
	for rows.Next() {
		var stage domain.SleepStage
		if err := rows.Scan(&stage.Stage, &stage.DurationMS); err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

func getWorkouts(ctx context.Context, database executor, date string) ([]domain.Workout, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT id, origin, external_id, device_kind, started_at, ended_at, discipline, duration_ms, distance_m, watch_verified, present
		FROM day_workout
		WHERE day_date = ?
		ORDER BY started_at, id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workouts := make([]domain.Workout, 0)
	for rows.Next() {
		var workout domain.Workout
		var deviceKind sql.NullString
		var distanceM sql.NullFloat64
		var watchVerified bool
		var present bool
		if err := rows.Scan(
			&workout.ID,
			&workout.Origin,
			&workout.ExternalID,
			&deviceKind,
			&workout.StartedAt,
			&workout.EndedAt,
			&workout.Discipline,
			&workout.DurationMS,
			&distanceM,
			&watchVerified,
			&present,
		); err != nil {
			return nil, err
		}
		workout.DeviceKind = nullableString(deviceKind)
		workout.DistanceM = nullableFloat64(distanceM)
		workout.WatchVerified = watchVerified
		workout.Present = present
		workouts = append(workouts, workout)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return workouts, nil
}

func saveDay(ctx context.Context, database executor, day domain.Day) error {
	if _, err := database.ExecContext(ctx, `
		INSERT INTO day (date, timezone, revision, sleep_ms, steps, resting_hr_bpm, hrv_rmssd_ms, updated_at, last_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			timezone = excluded.timezone,
			revision = excluded.revision,
			sleep_ms = excluded.sleep_ms,
			steps = excluded.steps,
			resting_hr_bpm = excluded.resting_hr_bpm,
			hrv_rmssd_ms = excluded.hrv_rmssd_ms,
			updated_at = excluded.updated_at,
			last_source = excluded.last_source`,
		day.Date,
		day.Timezone,
		day.Revision,
		int64Argument(day.SleepMS),
		int64Argument(day.Steps),
		int64Argument(day.RestingHRBPM),
		float64Argument(day.HRVRMSSDMS),
		day.UpdatedAt,
		day.LastSource,
	); err != nil {
		return err
	}

	if _, err := database.ExecContext(ctx, `DELETE FROM day_sleep_stage WHERE day_date = ?`, day.Date); err != nil {
		return err
	}
	for _, stage := range day.SleepStages {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO day_sleep_stage (day_date, stage, duration_ms)
			VALUES (?, ?, ?)`,
			day.Date,
			stage.Stage,
			stage.DurationMS,
		); err != nil {
			return err
		}
	}

	for _, workout := range day.Workouts {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO day_workout (id, day_date, origin, external_id, device_kind, started_at, ended_at, discipline, duration_ms, distance_m, watch_verified, present)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				day_date = excluded.day_date,
				origin = excluded.origin,
				external_id = excluded.external_id,
				device_kind = excluded.device_kind,
				started_at = excluded.started_at,
				ended_at = excluded.ended_at,
				discipline = excluded.discipline,
				duration_ms = excluded.duration_ms,
				distance_m = excluded.distance_m,
				watch_verified = excluded.watch_verified,
				present = excluded.present`,
			workout.ID,
			day.Date,
			workout.Origin,
			workout.ExternalID,
			stringArgument(workout.DeviceKind),
			workout.StartedAt,
			workout.EndedAt,
			workout.Discipline,
			workout.DurationMS,
			float64Argument(workout.DistanceM),
			workout.WatchVerified,
			workout.Present,
		); err != nil {
			return err
		}
	}

	return nil
}

func find(ctx context.Context, database executor, operation, key string) (app.Receipt, bool, error) {
	var receipt app.Receipt
	err := database.QueryRowContext(ctx, `
		SELECT request_hash, result_json
		FROM idempotency_request
		WHERE operation = ? AND key = ?`, operation, key).Scan(&receipt.RequestHash, &receipt.Result)
	if errors.Is(err, sql.ErrNoRows) {
		return app.Receipt{}, false, nil
	}
	if err != nil {
		return app.Receipt{}, false, err
	}
	return receipt, true, nil
}

func saveReceipt(ctx context.Context, database executor, operation, key, requestHash, result string, createdAt int64) error {
	_, err := database.ExecContext(ctx, `
		INSERT INTO idempotency_request (operation, key, request_hash, result_json, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		operation,
		key,
		requestHash,
		result,
		createdAt,
	)
	return err
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func nullableFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int64Argument(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func float64Argument(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringArgument(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
