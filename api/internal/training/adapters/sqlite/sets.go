package sqlite

import (
	"context"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (t *transaction) InsertSet(ctx context.Context, set domain.Set) error {
	_, err := t.query.ExecContext(ctx, `INSERT INTO logged_set (id, session_id, exercise_id, set_index, kg, reps, note, evidence_source, idempotency_key, created_at, revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		set.ID,
		set.SessionID,
		set.ExerciseID,
		set.SetIndex,
		set.KG,
		set.Reps,
		nullableString(set.Note),
		set.EvidenceSource,
		nullableString(set.IdempotencyKey),
		set.CreatedAt,
		set.Revision,
	)
	if isUniqueViolation(err, "logged_set.session_id, logged_set.exercise_id, logged_set.set_index") {
		return kernel.NewRuleError("set_position_occupied", "a set already exists at this session, exercise and position")
	}
	return err
}
