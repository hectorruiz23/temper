package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (t *transaction) ExerciseFacts(ctx context.Context, id string) (domain.ExerciseFacts, bool, error) {
	var unilateral sql.NullInt64
	err := t.query.QueryRowContext(ctx, `SELECT unilateral FROM exercise WHERE id = ?`, id).Scan(&unilateral)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ExerciseFacts{}, false, nil
	}
	if err != nil {
		return domain.ExerciseFacts{}, false, err
	}
	return domain.ExerciseFacts{ID: id, Unilateral: unilateral.Int64 == 1}, true, nil
}

func (t *transaction) PerformedSets(ctx context.Context, exerciseID string) ([]domain.PerformedSet, error) {
	rows, err := t.query.QueryContext(ctx, `SELECT ls.id, ls.session_id, ts.date, ls.set_index, ls.kg, ls.reps, ls.note, ls.evidence_source, ls.revision
		FROM logged_set ls
		JOIN training_session ts ON ts.id = ls.session_id
		WHERE ls.exercise_id = ?
		ORDER BY ls.created_at, ls.id`, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]domain.PerformedSet, 0)
	for rows.Next() {
		var (
			set  domain.PerformedSet
			note sql.NullString
		)
		if err := rows.Scan(&set.SetID, &set.SessionID, &set.Date, &set.SetIndex, &set.KG, &set.Reps, &note, &set.EvidenceSource, &set.Revision); err != nil {
			return nil, err
		}
		set.Note = nullableStringValue(note)
		history = append(history, set)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (t *transaction) CurrentTargets(ctx context.Context, exerciseID string) ([]domain.ExerciseTarget, error) {
	var planID int64
	err := t.query.QueryRowContext(ctx, `SELECT id FROM plan_version ORDER BY effective_from DESC, id DESC LIMIT 1`).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return make([]domain.ExerciseTarget, 0), nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := t.query.QueryContext(ctx, `SELECT se.template_id, se.target_sets, se.reps_min, se.reps_max, se.target_kg, se.tempo
		FROM session_exercise se
		JOIN session_template st ON st.plan_version_id = se.plan_version_id AND st.id = se.template_id
		WHERE st.plan_version_id = ? AND se.exercise_id = ?
		ORDER BY st.position, st.id, se.position`, planID, exerciseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]domain.ExerciseTarget, 0)
	for rows.Next() {
		var (
			target     domain.ExerciseTarget
			targetSets sql.NullInt64
			repsMin    sql.NullInt64
			repsMax    sql.NullInt64
			targetKG   sql.NullFloat64
			tempo      sql.NullString
		)
		if err := rows.Scan(&target.TemplateID, &targetSets, &repsMin, &repsMax, &targetKG, &tempo); err != nil {
			return nil, err
		}
		target.TargetSets = nullableIntValue(targetSets)
		target.RepsMin = nullableIntValue(repsMin)
		target.RepsMax = nullableIntValue(repsMax)
		target.TargetKG = nullableFloatValue(targetKG)
		target.Tempo = nullableStringValue(tempo)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func nullableIntValue(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableFloatValue(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}
