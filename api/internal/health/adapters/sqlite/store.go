package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

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
	var stepsSource sql.NullString

	err := database.QueryRowContext(ctx, `
		SELECT date, timezone, revision, sleep_ms, steps, resting_hr_bpm, hrv_rmssd_ms, updated_at, last_source, steps_source
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
		&stepsSource,
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
	day.StepsSource = nullableString(stepsSource)

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
		SELECT id, origin, external_id, device_kind, started_at, ended_at, discipline, duration_ms, distance_m,
			avg_hr_bpm, max_hr_bpm, calories_kcal, cadence_spm, elevation_gain_m, pace_sec_per_km, watch_verified, present
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
		var avgHR sql.NullInt64
		var maxHR sql.NullInt64
		var calories sql.NullFloat64
		var cadence sql.NullFloat64
		var elevation sql.NullFloat64
		var pace sql.NullFloat64
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
			&avgHR,
			&maxHR,
			&calories,
			&cadence,
			&elevation,
			&pace,
			&watchVerified,
			&present,
		); err != nil {
			return nil, err
		}
		workout.DeviceKind = nullableString(deviceKind)
		workout.DistanceM = nullableFloat64(distanceM)
		workout.AvgHRBPM = nullableInt64(avgHR)
		workout.MaxHRBPM = nullableInt64(maxHR)
		workout.CaloriesKcal = nullableFloat64(calories)
		workout.CadenceSPM = nullableFloat64(cadence)
		workout.ElevationGainM = nullableFloat64(elevation)
		workout.PaceSecPerKM = nullableFloat64(pace)
		workout.WatchVerified = watchVerified
		workout.Present = present
		workouts = append(workouts, workout)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachSplits(ctx, database, workouts); err != nil {
		return nil, err
	}
	return workouts, nil
}

func attachSplits(ctx context.Context, database executor, workouts []domain.Workout) error {
	if len(workouts) == 0 {
		return nil
	}
	byID := make(map[string]*domain.Workout, len(workouts))
	arguments := make([]any, 0, len(workouts))
	for index := range workouts {
		byID[workouts[index].ID] = &workouts[index]
		arguments = append(arguments, workouts[index].ID)
	}
	query := `SELECT workout_id, position, kind, distance_m, duration_ms, avg_hr_bpm, pace_sec_per_km
		FROM workout_split
		WHERE workout_id IN (` + placeholders(len(workouts)) + `)
		ORDER BY workout_id, position`
	rows, err := database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var workoutID string
		var split domain.WorkoutSplit
		var kind sql.NullString
		var distance sql.NullFloat64
		var duration sql.NullInt64
		var avgHR sql.NullInt64
		var pace sql.NullFloat64
		if err := rows.Scan(&workoutID, &split.Position, &kind, &distance, &duration, &avgHR, &pace); err != nil {
			return err
		}
		split.Kind = nullableString(kind)
		split.DistanceM = nullableFloat64(distance)
		split.DurationMS = nullableInt64(duration)
		split.AvgHRBPM = nullableInt64(avgHR)
		split.PaceSecPerKM = nullableFloat64(pace)
		workout, ok := byID[workoutID]
		if !ok {
			continue
		}
		workout.Splits = append(workout.Splits, split)
	}
	return rows.Err()
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", count), ", ")
}

func saveDay(ctx context.Context, database executor, day domain.Day) error {
	if _, err := database.ExecContext(ctx, `
		INSERT INTO day (date, timezone, revision, sleep_ms, steps, resting_hr_bpm, hrv_rmssd_ms, updated_at, last_source, steps_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			timezone = excluded.timezone,
			revision = excluded.revision,
			sleep_ms = excluded.sleep_ms,
			steps = excluded.steps,
			resting_hr_bpm = excluded.resting_hr_bpm,
			hrv_rmssd_ms = excluded.hrv_rmssd_ms,
			updated_at = excluded.updated_at,
			last_source = excluded.last_source,
			steps_source = excluded.steps_source`,
		day.Date,
		day.Timezone,
		day.Revision,
		int64Argument(day.SleepMS),
		int64Argument(day.Steps),
		int64Argument(day.RestingHRBPM),
		float64Argument(day.HRVRMSSDMS),
		day.UpdatedAt,
		day.LastSource,
		stringArgument(day.StepsSource),
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
			INSERT INTO day_workout (id, day_date, origin, external_id, device_kind, started_at, ended_at, discipline, duration_ms, distance_m,
				avg_hr_bpm, max_hr_bpm, calories_kcal, cadence_spm, elevation_gain_m, pace_sec_per_km, watch_verified, present)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
				avg_hr_bpm = excluded.avg_hr_bpm,
				max_hr_bpm = excluded.max_hr_bpm,
				calories_kcal = excluded.calories_kcal,
				cadence_spm = excluded.cadence_spm,
				elevation_gain_m = excluded.elevation_gain_m,
				pace_sec_per_km = excluded.pace_sec_per_km,
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
			int64Argument(workout.AvgHRBPM),
			int64Argument(workout.MaxHRBPM),
			float64Argument(workout.CaloriesKcal),
			float64Argument(workout.CadenceSPM),
			float64Argument(workout.ElevationGainM),
			float64Argument(workout.PaceSecPerKM),
			workout.WatchVerified,
			workout.Present,
		); err != nil {
			return err
		}
		if !workout.Present {
			continue
		}
		if _, err := database.ExecContext(ctx, `DELETE FROM workout_split WHERE workout_id = ?`, workout.ID); err != nil {
			return err
		}
		for _, split := range workout.Splits {
			if _, err := database.ExecContext(ctx, `
				INSERT INTO workout_split (workout_id, position, kind, distance_m, duration_ms, avg_hr_bpm, pace_sec_per_km)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				workout.ID,
				split.Position,
				stringArgument(split.Kind),
				float64Argument(split.DistanceM),
				int64Argument(split.DurationMS),
				int64Argument(split.AvgHRBPM),
				float64Argument(split.PaceSecPerKM),
			); err != nil {
				return err
			}
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
