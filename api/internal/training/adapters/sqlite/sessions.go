package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (t *transaction) Template(ctx context.Context, id string) (string, bool, error) {
	var discipline sql.NullString
	err := t.query.QueryRowContext(ctx, `SELECT discipline FROM session_template WHERE id = ?`, id).Scan(&discipline)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return discipline.String, true, nil
}

func (t *transaction) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := t.query.ExecContext(ctx, `INSERT INTO training_session (id, template_id, date, discipline, started_at, reported_completed_at, watch_workout_id, created_at, revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		nullableString(session.TemplateID),
		session.Date,
		session.Discipline,
		nullableInt64(session.StartedAt),
		nullableInt64(session.ReportedCompletedAt),
		nullableString(session.WatchWorkoutID),
		session.CreatedAt,
		session.Revision,
	)
	if isUniqueViolation(err, "training_session.id") {
		return kernel.NewRuleError("session_exists", "a session with this id already exists")
	}
	return err
}

func (t *transaction) Session(ctx context.Context, id string) (domain.Session, error) {
	var (
		session    domain.Session
		templateID sql.NullString
		startedAt  sql.NullInt64
		reportedAt sql.NullInt64
		watchID    sql.NullString
		present    int
	)
	err := t.query.QueryRowContext(ctx, `SELECT ts.id, ts.template_id, ts.date, ts.discipline, ts.started_at, ts.reported_completed_at, ts.watch_workout_id, ts.created_at, ts.revision, COALESCE(dw.present, 0)
		FROM training_session ts
		LEFT JOIN day_workout dw ON dw.id = ts.watch_workout_id
		WHERE ts.id = ?`, id).
		Scan(&session.ID, &templateID, &session.Date, &session.Discipline, &startedAt, &reportedAt, &watchID, &session.CreatedAt, &session.Revision, &present)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, kernel.NewRuleError("not_found", "session not found")
	}
	if err != nil {
		return domain.Session{}, err
	}

	session.TemplateID = nullableStringValue(templateID)
	session.StartedAt = nullableInt64Value(startedAt)
	session.ReportedCompletedAt = nullableInt64Value(reportedAt)
	session.WatchWorkoutID = nullableStringValue(watchID)
	session.Complete = domain.IsComplete(session.WatchWorkoutID, present == 1, session.ReportedCompletedAt)
	return session, nil
}

func (s *Store) SessionsBetween(ctx context.Context, startDate, endDate string) ([]domain.Session, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT ts.id, ts.template_id, ts.date, ts.discipline, ts.started_at, ts.reported_completed_at, ts.watch_workout_id, ts.created_at, ts.revision, COALESCE(dw.present, 0)
		FROM training_session ts
		LEFT JOIN day_workout dw ON dw.id = ts.watch_workout_id
		WHERE ts.date >= ? AND ts.date <= ?
		ORDER BY ts.date, ts.started_at, ts.id`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]domain.Session, 0)
	for rows.Next() {
		var (
			session    domain.Session
			templateID sql.NullString
			startedAt  sql.NullInt64
			reportedAt sql.NullInt64
			watchID    sql.NullString
			present    int
		)
		if err := rows.Scan(&session.ID, &templateID, &session.Date, &session.Discipline, &startedAt, &reportedAt, &watchID, &session.CreatedAt, &session.Revision, &present); err != nil {
			return nil, err
		}

		session.TemplateID = nullableStringValue(templateID)
		session.StartedAt = nullableInt64Value(startedAt)
		session.ReportedCompletedAt = nullableInt64Value(reportedAt)
		session.WatchWorkoutID = nullableStringValue(watchID)
		session.Complete = domain.IsComplete(session.WatchWorkoutID, present == 1, session.ReportedCompletedAt)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (t *transaction) SetCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	if err := t.query.QueryRowContext(ctx, `SELECT COUNT(*) FROM logged_set WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (t *transaction) DeleteSession(ctx context.Context, id string) error {
	result, err := t.query.ExecContext(ctx, `DELETE FROM training_session WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return kernel.NewRuleError("not_found", "session not found")
	}
	return nil
}

func (t *transaction) UpdateSessionEvidence(ctx context.Context, id string, expectedRevision, nextRevision int64, watchWorkoutID *string, reportedCompletedAt *int64) error {
	result, err := t.query.ExecContext(ctx, `UPDATE training_session
		SET watch_workout_id = ?, reported_completed_at = ?, revision = ?
		WHERE id = ? AND revision = ?`,
		nullableString(watchWorkoutID),
		nullableInt64(reportedCompletedAt),
		nextRevision,
		id,
		expectedRevision,
	)
	if isUniqueViolation(err, "training_session.watch_workout_id") {
		return kernel.NewRuleError("workout_already_linked", "watch workout is already linked to another session")
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return kernel.NewRuleError("revision_conflict", "expected_revision does not match the stored revision")
	}
	return nil
}

func (t *transaction) Workout(ctx context.Context, id string) (*domain.Workout, error) {
	var (
		workout       domain.Workout
		present       int
		watchVerified int
	)
	err := t.query.QueryRowContext(ctx, `SELECT id, present, watch_verified FROM day_workout WHERE id = ?`, id).
		Scan(&workout.ID, &present, &watchVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	workout.Present = present == 1
	workout.WatchVerified = watchVerified == 1
	return &workout, nil
}

func (t *transaction) ExerciseExists(ctx context.Context, id string) (bool, error) {
	var count int
	if err := t.query.QueryRowContext(ctx, `SELECT COUNT(*) FROM exercise WHERE id = ?`, id).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func isUniqueViolation(err error, fragment string) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed") && strings.Contains(message, fragment)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableInt64Value(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
