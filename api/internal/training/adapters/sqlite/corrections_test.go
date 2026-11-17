package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/training/app"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func trainingFloatPtr(value float64) *float64 {
	return &value
}

func seedCorrectableExercise(t *testing.T, database *sql.DB, id string) {
	t.Helper()

	seedTrainingExercise(t, database, id)
	if _, err := database.Exec(`INSERT INTO exercise_equipment (exercise_id, equipment_id, quantity) VALUES (?, 'adjustable-dumbbell', 1)`, id); err != nil {
		t.Fatalf("insert exercise equipment: %v", err)
	}
}

func logCorrectionSet(t *testing.T, service *app.Service, sessionID, exerciseID string, index int, kg float64, reps int, note *string) domain.Set {
	t.Helper()

	result, err := service.LogSets(context.Background(), app.LogSetsCommand{
		IdempotencyKey: fmt.Sprintf("log-%s-%d", exerciseID, index),
		SessionID:      sessionID,
		Sets:           []domain.NewSet{{ExerciseID: exerciseID, SetIndex: index, KG: kg, Reps: reps, Note: note}},
	})
	if err != nil {
		t.Fatalf("log set: %v", err)
	}
	return result.Sets[0]
}

func readLoggedSet(t *testing.T, database *sql.DB, id string) (revision int64, setIndex int, kg float64, reps int) {
	t.Helper()

	if err := database.QueryRow(`SELECT revision, set_index, kg, reps FROM logged_set WHERE id = ?`, id).Scan(&revision, &setIndex, &kg, &reps); err != nil {
		t.Fatalf("read logged set: %v", err)
	}
	return revision, setIndex, kg, reps
}

func TestCorrectSetInsertsCorrectionAndBumpsRevision(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedCorrectableExercise(t, database, "goblet-squat")
	session := newTrainingSession(t, service, "session-1")

	original := logCorrectionSet(t, service, session.ID, "goblet-squat", 1, 20, 10, trainingStringPtr("old"))
	before, err := domain.CanonicalSetJSON(original)
	if err != nil {
		t.Fatalf("canonical before: %v", err)
	}

	first, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "correct-1",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "  adjusted reps  ",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(12)},
	})
	if err != nil {
		t.Fatalf("correct set: %v", err)
	}
	if first.Set.Revision != 2 || first.Set.Reps != 12 {
		t.Fatalf("corrected set = %+v", first.Set)
	}
	if first.CorrectionID == "" || first.Echo.ExpectedRevision != 1 || first.Echo.Reason != "adjusted reps" {
		t.Fatalf("correction result = %+v", first)
	}

	if revision, _, _, reps := readLoggedSet(t, database, original.ID); revision != 2 || reps != 12 {
		t.Fatalf("stored revision = %d reps = %d, want 2 and 12", revision, reps)
	}

	var (
		correctionID string
		resulting    int64
		storedBefore string
		storedAfter  string
	)
	if err := database.QueryRow(`SELECT id, resulting_revision, before_json, after_json FROM set_correction WHERE set_id = ?`, original.ID).
		Scan(&correctionID, &resulting, &storedBefore, &storedAfter); err != nil {
		t.Fatalf("read correction: %v", err)
	}
	if correctionID != first.CorrectionID || resulting != 2 {
		t.Fatalf("stored correction id = %q resulting = %d", correctionID, resulting)
	}
	if storedBefore != before {
		t.Fatalf("before_json = %s, want %s", storedBefore, before)
	}
	after, err := domain.CanonicalSetJSON(first.Set)
	if err != nil {
		t.Fatalf("canonical after: %v", err)
	}
	if storedAfter != after {
		t.Fatalf("after_json = %s, want %s", storedAfter, after)
	}

	second, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "correct-2",
		SetID:            original.ID,
		ExpectedRevision: 2,
		Reason:           "second adjustment",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(11)},
	})
	if err != nil {
		t.Fatalf("second correction: %v", err)
	}
	if second.Set.Revision != 3 {
		t.Fatalf("second revision = %d, want 3", second.Set.Revision)
	}

	rows, err := database.Query(`SELECT resulting_revision FROM set_correction WHERE set_id = ? ORDER BY resulting_revision`, original.ID)
	if err != nil {
		t.Fatalf("query corrections: %v", err)
	}
	defer rows.Close()
	var revisions []int64
	for rows.Next() {
		var revision int64
		if err := rows.Scan(&revision); err != nil {
			t.Fatalf("scan revision: %v", err)
		}
		revisions = append(revisions, revision)
	}
	if !reflect.DeepEqual(revisions, []int64{2, 3}) {
		t.Fatalf("resulting revisions = %v, want [2 3]", revisions)
	}
}

func TestCorrectSetRejectsConflictsAndInvalidValues(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedCorrectableExercise(t, database, "goblet-squat")
	session := newTrainingSession(t, service, "session-1")

	original := logCorrectionSet(t, service, session.ID, "goblet-squat", 1, 20, 10, nil)
	logCorrectionSet(t, service, session.ID, "goblet-squat", 2, 20, 8, nil)

	_, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "stale",
		SetID:            original.ID,
		ExpectedRevision: 2,
		Reason:           "stale",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(11)},
	})
	requireTrainingCode(t, err, "revision_conflict")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "occupied",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "occupied",
		Changes:          domain.SetChanges{SetIndex: trainingIntPtr(2)},
	})
	requireTrainingCode(t, err, "set_position_occupied")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "limit",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "too heavy",
		Changes:          domain.SetChanges{KG: trainingFloatPtr(25)},
	})
	requireTrainingCode(t, err, "equipment_limit")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "unknown",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "unknown exercise",
		Changes:          domain.SetChanges{ExerciseID: trainingStringPtr("missing")},
	})
	requireTrainingCode(t, err, "unknown_exercise")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "empty-reason",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "   ",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(11)},
	})
	requireTrainingCode(t, err, "invalid_input")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "no-changes",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "nothing",
		Changes:          domain.SetChanges{},
	})
	requireTrainingCode(t, err, "invalid_input")

	_, err = service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "missing-set",
		SetID:            "missing",
		ExpectedRevision: 1,
		Reason:           "missing",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(11)},
	})
	requireTrainingCode(t, err, "not_found")

	if revision, setIndex, kg, reps := readLoggedSet(t, database, original.ID); revision != 1 || setIndex != 1 || kg != 20 || reps != 10 {
		t.Fatalf("set changed after rejected corrections: revision=%d index=%d kg=%g reps=%d", revision, setIndex, kg, reps)
	}
	if count := countTrainingRows(t, database, "set_correction"); count != 0 {
		t.Fatalf("set_correction rows = %d, want 0", count)
	}
}

func TestCorrectSetReplayReturnsStoredResult(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedCorrectableExercise(t, database, "goblet-squat")
	session := newTrainingSession(t, service, "session-1")

	original := logCorrectionSet(t, service, session.ID, "goblet-squat", 1, 20, 10, nil)
	command := app.CorrectSetCommand{
		IdempotencyKey:   "correct-key",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "reps",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(12)},
	}

	first, err := service.CorrectSet(ctx, command)
	if err != nil {
		t.Fatalf("correct set: %v", err)
	}
	replayed, err := service.CorrectSet(ctx, command)
	if err != nil {
		t.Fatalf("replay correction: %v", err)
	}
	if !reflect.DeepEqual(replayed, first) {
		t.Fatalf("replay = %+v, want %+v", replayed, first)
	}
	if count := countTrainingRows(t, database, "set_correction"); count != 1 {
		t.Fatalf("set_correction rows = %d after replay, want 1", count)
	}

	changed := command
	changed.Changes = domain.SetChanges{Reps: trainingIntPtr(9)}
	_, err = service.CorrectSet(ctx, changed)
	requireTrainingCode(t, err, "idempotency_conflict")

	second, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "correct-key-2",
		SetID:            original.ID,
		ExpectedRevision: 2,
		Reason:           "again",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(11)},
	})
	if err != nil {
		t.Fatalf("second correction: %v", err)
	}
	if second.Set.Revision != 3 {
		t.Fatalf("second revision = %d, want 3", second.Set.Revision)
	}

	replayed, err = service.CorrectSet(ctx, command)
	if err != nil {
		t.Fatalf("replay after second correction: %v", err)
	}
	if !reflect.DeepEqual(replayed, first) {
		t.Fatalf("late replay = %+v, want %+v", replayed, first)
	}
	if count := countTrainingRows(t, database, "set_correction"); count != 2 {
		t.Fatalf("set_correction rows = %d, want 2", count)
	}
}

func TestCorrectSetClearsNoteAndAllowsNoOp(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedCorrectableExercise(t, database, "goblet-squat")
	session := newTrainingSession(t, service, "session-1")

	original := logCorrectionSet(t, service, session.ID, "goblet-squat", 1, 20, 10, trainingStringPtr("old"))

	cleared, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "clear-note",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "blank note",
		Changes:          domain.SetChanges{Note: trainingStringPtr("   ")},
	})
	if err != nil {
		t.Fatalf("clear note: %v", err)
	}
	if cleared.Set.Note != nil {
		t.Fatalf("note = %q, want nil", *cleared.Set.Note)
	}

	noop, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "noop",
		SetID:            original.ID,
		ExpectedRevision: 2,
		Reason:           "restated",
		Changes:          domain.SetChanges{KG: trainingFloatPtr(20)},
	})
	if err != nil {
		t.Fatalf("no-op correction: %v", err)
	}
	if noop.Set.Revision != 3 || noop.Set.KG != 20 {
		t.Fatalf("no-op set = %+v", noop.Set)
	}
}

func TestProgressionReadsCorrectedHistoryAndPlanTargets(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedCorrectableExercise(t, database, "goblet-squat")
	seedTrainingTemplate(t, database, "template-strength", domain.DisciplineStrength)
	if _, err := database.Exec(`INSERT INTO session_exercise (plan_version_id, template_id, position, exercise_id, target_sets, reps_min, reps_max, target_kg, tempo)
		SELECT plan_version_id, 'template-strength', 0, 'goblet-squat', 3, 8, 12, 20, '3-1-1' FROM session_template WHERE id = 'template-strength'`); err != nil {
		t.Fatalf("insert session exercise: %v", err)
	}
	session := newTrainingSession(t, service, "session-1")

	original := logCorrectionSet(t, service, session.ID, "goblet-squat", 1, 20, 10, nil)
	if _, err := service.CorrectSet(ctx, app.CorrectSetCommand{
		IdempotencyKey:   "correct",
		SetID:            original.ID,
		ExpectedRevision: 1,
		Reason:           "reps",
		Changes:          domain.SetChanges{Reps: trainingIntPtr(12)},
	}); err != nil {
		t.Fatalf("correct set: %v", err)
	}

	progression, err := service.Progression(ctx, "goblet-squat")
	if err != nil {
		t.Fatalf("progression: %v", err)
	}
	if len(progression.PerformedHistory) != 1 {
		t.Fatalf("history = %+v", progression.PerformedHistory)
	}
	set := progression.PerformedHistory[0]
	if set.SetID != original.ID || set.SessionID != session.ID || set.Reps != 12 || set.Revision != 2 || set.Date != "2026-12-02" {
		t.Fatalf("history set = %+v", set)
	}
	if len(progression.CurrentTargets) != 1 {
		t.Fatalf("targets = %+v", progression.CurrentTargets)
	}
	target := progression.CurrentTargets[0]
	if target.TemplateID != "template-strength" || target.TargetSets == nil || *target.TargetSets != 3 || target.RepsMax == nil || *target.RepsMax != 12 || target.TargetKG == nil || *target.TargetKG != 20 {
		t.Fatalf("target = %+v", target)
	}
	if len(progression.ProgressionDimensions) != 3 {
		t.Fatalf("dimensions = %+v", progression.ProgressionDimensions)
	}

	_, err = service.Progression(ctx, "missing")
	requireTrainingCode(t, err, "not_found")
}
