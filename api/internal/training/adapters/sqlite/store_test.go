package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/internal/training/app"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
	"github.com/hectorruiz23/temper/api/migrations"
)

type trainingFixedClock struct {
	now time.Time
}

func (c trainingFixedClock) Now() time.Time {
	return c.now
}

func trainingIntPtr(value int) *int {
	return &value
}

func trainingInt64Ptr(value int64) *int64 {
	return &value
}

func trainingStringPtr(value string) *string {
	return &value
}

func newTrainingTestService(t *testing.T) (*app.Service, *sql.DB) {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "training.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	clock := trainingFixedClock{now: time.Unix(1763300000, 0)}
	return app.New(New(database), clock), database
}

func seedTrainingTemplate(t *testing.T, database *sql.DB, id, discipline string) {
	t.Helper()

	result, err := database.Exec(`INSERT INTO plan_version (reason, effective_from, timezone, weekly_frequency) VALUES ('test', '2026-12-01', 'UTC', 1)`)
	if err != nil {
		t.Fatalf("insert plan version: %v", err)
	}
	planID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("plan id: %v", err)
	}

	if _, err := database.Exec(`INSERT INTO session_template (id, plan_version_id, position, scheduled_date, discipline) VALUES (?, ?, 0, '2026-12-02', ?)`, id, planID, discipline); err != nil {
		t.Fatalf("insert session template: %v", err)
	}
}

func seedTrainingExercise(t *testing.T, database *sql.DB, id string) {
	t.Helper()

	if _, err := database.Exec(`INSERT INTO exercise (id, name, movement_pattern) VALUES (?, ?, 'squat')`, id, id); err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
}

func seedTrainingWorkout(t *testing.T, database *sql.DB, id string, present, verified bool) {
	t.Helper()

	if _, err := database.Exec(`INSERT OR IGNORE INTO day (date, timezone, revision, updated_at, last_source) VALUES ('2026-12-02', 'UTC', 1, 1, 'health_connect')`); err != nil {
		t.Fatalf("insert day: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO day_workout (id, day_date, origin, external_id, started_at, ended_at, discipline, duration_ms, watch_verified, present)
		VALUES (?, '2026-12-02', 'health_connect', ?, 1, 2, 'run', 1000, ?, ?)`, id, id, verified, present); err != nil {
		t.Fatalf("insert workout: %v", err)
	}
}

func requireTrainingCode(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want code %q", want)
	}
	var rule kernel.RuleError
	if !errors.As(err, &rule) {
		t.Fatalf("error %v is not a kernel.RuleError", err)
	}
	if rule.Code != want {
		t.Fatalf("code = %q, want %q (message %q)", rule.Code, want, rule.Message)
	}
}

func countTrainingRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func newTrainingSession(t *testing.T, service *app.Service, id string) domain.Session {
	t.Helper()

	session, err := service.CreateSession(context.Background(), app.CreateSessionCommand{
		IdempotencyKey: "create-" + id,
		ID:             id,
		Date:           "2026-12-02",
		Discipline:     domain.DisciplineStrength,
	})
	if err != nil {
		t.Fatalf("create session %s: %v", id, err)
	}
	return session
}

func TestCreateSessionWritesNoEvidence(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingTemplate(t, database, "template-strength", domain.DisciplineStrength)

	template := "template-strength"
	started := time.Unix(1763300100, 0).UnixMilli()
	command := app.CreateSessionCommand{
		IdempotencyKey: "session-key",
		ID:             "session-1",
		TemplateID:     &template,
		Date:           "2026-12-02",
		Discipline:     domain.DisciplineStrength,
		StartedAt:      &started,
	}

	created, err := service.CreateSession(ctx, command)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.ID != "session-1" || created.Revision != 1 || created.Complete {
		t.Fatalf("created session = %+v", created)
	}
	if created.WatchWorkoutID != nil || created.ReportedCompletedAt != nil {
		t.Fatalf("created session carries evidence: %+v", created)
	}
	if created.TemplateID == nil || *created.TemplateID != "template-strength" {
		t.Fatalf("template id = %v", created.TemplateID)
	}

	var reported sql.NullInt64
	var watch sql.NullString
	if err := database.QueryRow(`SELECT reported_completed_at, watch_workout_id FROM training_session WHERE id = ?`, "session-1").Scan(&reported, &watch); err != nil {
		t.Fatalf("read session row: %v", err)
	}
	if reported.Valid || watch.Valid {
		t.Fatalf("stored evidence reported=%v watch=%v, want both null", reported, watch)
	}
	if count := countTrainingRows(t, database, "logged_set"); count != 0 {
		t.Fatalf("logged_set rows = %d, want 0", count)
	}

	replayed, err := service.CreateSession(ctx, command)
	if err != nil {
		t.Fatalf("replay create session: %v", err)
	}
	if !reflect.DeepEqual(replayed, created) {
		t.Fatalf("replay = %+v, want %+v", replayed, created)
	}
	if count := countTrainingRows(t, database, "training_session"); count != 1 {
		t.Fatalf("training_session rows = %d after replay, want 1", count)
	}

	changed := command
	changed.Date = "2026-12-03"
	_, err = service.CreateSession(ctx, changed)
	requireTrainingCode(t, err, "idempotency_conflict")

	repeated := command
	repeated.IdempotencyKey = "another-key"
	_, err = service.CreateSession(ctx, repeated)
	requireTrainingCode(t, err, "session_exists")

	unknown := command
	unknown.IdempotencyKey = "unknown-template-key"
	unknown.ID = "session-2"
	missing := "missing-template"
	unknown.TemplateID = &missing
	_, err = service.CreateSession(ctx, unknown)
	requireTrainingCode(t, err, "unknown_template")

	mismatch := command
	mismatch.IdempotencyKey = "mismatch-key"
	mismatch.ID = "session-3"
	mismatch.Discipline = domain.DisciplineRun
	_, err = service.CreateSession(ctx, mismatch)
	requireTrainingCode(t, err, "template_mismatch")
}

func TestCheckInWithEachEvidenceSource(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingWorkout(t, database, "watch-1", true, true)
	seedTrainingWorkout(t, database, "watch-unverified", true, false)

	reported := newTrainingSession(t, service, "session-reported")
	first, err := service.CheckIn(ctx, app.CheckInCommand{
		SessionID:           reported.ID,
		ExpectedRevision:    1,
		ReportedCompletedAt: trainingInt64Ptr(1763304000000),
	})
	if err != nil {
		t.Fatalf("check in reported: %v", err)
	}
	if !first.Session.Complete || first.Session.Revision != 2 {
		t.Fatalf("reported session = %+v", first.Session)
	}
	if first.Evidence == nil || first.Evidence.Source != domain.EvidenceReported || first.Evidence.ReportedCompletedAt == nil {
		t.Fatalf("reported evidence = %+v", first.Evidence)
	}

	watched := newTrainingSession(t, service, "session-watched")
	second, err := service.CheckIn(ctx, app.CheckInCommand{
		SessionID:        watched.ID,
		ExpectedRevision: 1,
		WatchWorkoutID:   trainingStringPtr("watch-1"),
	})
	if err != nil {
		t.Fatalf("check in watch: %v", err)
	}
	if !second.Session.Complete || second.Session.WatchWorkoutID == nil || *second.Session.WatchWorkoutID != "watch-1" {
		t.Fatalf("watched session = %+v", second.Session)
	}
	if second.Evidence == nil || second.Evidence.Source != domain.EvidenceWatch || second.Evidence.WatchWorkoutID == nil {
		t.Fatalf("watch evidence = %+v", second.Evidence)
	}

	if _, err := service.CheckIn(ctx, app.CheckInCommand{
		SessionID:           reported.ID,
		ExpectedRevision:    1,
		ReportedCompletedAt: trainingInt64Ptr(1763304000000),
	}); err != nil {
		requireTrainingCode(t, err, "revision_conflict")
	}

	_, err = service.CheckIn(ctx, app.CheckInCommand{SessionID: "missing", ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("watch-1")})
	requireTrainingCode(t, err, "not_found")

	unverified := newTrainingSession(t, service, "session-unverified")
	_, err = service.CheckIn(ctx, app.CheckInCommand{SessionID: unverified.ID, ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("watch-unverified")})
	requireTrainingCode(t, err, "unusable_watch_evidence")
	_, err = service.CheckIn(ctx, app.CheckInCommand{SessionID: unverified.ID, ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("missing-watch")})
	requireTrainingCode(t, err, "unusable_watch_evidence")

	replaced := newTrainingSession(t, service, "session-replaced")
	if _, err := service.CheckIn(ctx, app.CheckInCommand{SessionID: replaced.ID, ExpectedRevision: 1, ReportedCompletedAt: trainingInt64Ptr(1763304000000)}); err != nil {
		t.Fatalf("first evidence: %v", err)
	}
	seedTrainingWorkout(t, database, "watch-2", true, true)
	switched, err := service.CheckIn(ctx, app.CheckInCommand{SessionID: replaced.ID, ExpectedRevision: 2, WatchWorkoutID: trainingStringPtr("watch-2")})
	if err != nil {
		t.Fatalf("switch to watch evidence: %v", err)
	}
	if switched.Session.ReportedCompletedAt != nil || switched.Session.WatchWorkoutID == nil {
		t.Fatalf("session keeps both evidence sources: %+v", switched.Session)
	}
}

func TestCheckInWorkoutLinkedTwice(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingWorkout(t, database, "watch-1", true, true)

	first := newTrainingSession(t, service, "session-first")
	if _, err := service.CheckIn(ctx, app.CheckInCommand{SessionID: first.ID, ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("watch-1")}); err != nil {
		t.Fatalf("first link: %v", err)
	}

	second := newTrainingSession(t, service, "session-second")
	_, err := service.CheckIn(ctx, app.CheckInCommand{SessionID: second.ID, ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("watch-1")})
	requireTrainingCode(t, err, "workout_already_linked")

	var watch sql.NullString
	var revision int64
	if err := database.QueryRow(`SELECT watch_workout_id, revision FROM training_session WHERE id = ?`, second.ID).Scan(&watch, &revision); err != nil {
		t.Fatalf("read second session: %v", err)
	}
	if watch.Valid || revision != 1 {
		t.Fatalf("second session watch=%v revision=%d after conflict", watch, revision)
	}
}

func TestLogSetsBatchIsAtomicAndIdempotent(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingExercise(t, database, "goblet-squat")
	seedTrainingExercise(t, database, "push-up")
	session := newTrainingSession(t, service, "session-1")

	note := "  felt good  "
	command := app.LogSetsCommand{
		IdempotencyKey: "batch-1",
		SessionID:      session.ID,
		Sets: []domain.NewSet{
			{ExerciseID: "goblet-squat", SetIndex: 1, KG: 20.005, Reps: 10, Note: &note},
			{ExerciseID: "push-up", SetIndex: 1, KG: 0, Reps: 12, Note: trainingStringPtr("   ")},
		},
	}

	result, err := service.LogSets(ctx, command)
	if err != nil {
		t.Fatalf("log sets: %v", err)
	}
	if len(result.Sets) != 2 || len(result.Echo.Sets) != 2 {
		t.Fatalf("batch result = %+v", result)
	}
	first := result.Sets[0]
	if first.ExerciseID != "goblet-squat" || first.KG != 20.01 || first.Reps != 10 {
		t.Fatalf("first set = %+v", first)
	}
	if first.Note == nil || *first.Note != "felt good" {
		t.Fatalf("first note = %v", first.Note)
	}
	if first.EvidenceSource != domain.EvidenceReported || first.Revision != 1 || first.ID == "" {
		t.Fatalf("first set metadata = %+v", first)
	}
	if result.Sets[1].Note != nil {
		t.Fatalf("blank note = %v, want nil", *result.Sets[1].Note)
	}

	var storedKey string
	if err := database.QueryRow(`SELECT idempotency_key FROM logged_set WHERE id = ?`, first.ID).Scan(&storedKey); err != nil {
		t.Fatalf("read idempotency key: %v", err)
	}
	if storedKey != "batch-1" {
		t.Fatalf("stored idempotency key = %q", storedKey)
	}

	replayed, err := service.LogSets(ctx, command)
	if err != nil {
		t.Fatalf("replay batch: %v", err)
	}
	if !reflect.DeepEqual(replayed, result) {
		t.Fatalf("replay = %+v, want %+v", replayed, result)
	}
	if count := countTrainingRows(t, database, "logged_set"); count != 2 {
		t.Fatalf("logged_set rows = %d after replay, want 2", count)
	}

	changed := command
	changed.Sets = append([]domain.NewSet(nil), command.Sets...)
	changed.Sets[0].Reps = 9
	_, err = service.LogSets(ctx, changed)
	requireTrainingCode(t, err, "idempotency_conflict")

	occupied := app.LogSetsCommand{
		IdempotencyKey: "batch-2",
		SessionID:      session.ID,
		Sets:           []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 1, KG: 20, Reps: 10}},
	}
	_, err = service.LogSets(ctx, occupied)
	requireTrainingCode(t, err, "set_position_occupied")
	if count := countTrainingRows(t, database, "logged_set"); count != 2 {
		t.Fatalf("logged_set rows = %d after occupied position, want 2", count)
	}

	rollback := app.LogSetsCommand{
		IdempotencyKey: "batch-3",
		SessionID:      session.ID,
		Sets: []domain.NewSet{
			{ExerciseID: "goblet-squat", SetIndex: 2, KG: 20, Reps: 10},
			{ExerciseID: "missing", SetIndex: 1, KG: 0, Reps: 10},
		},
	}
	_, err = service.LogSets(ctx, rollback)
	requireTrainingCode(t, err, "unknown_exercise")
	if count := countTrainingRows(t, database, "logged_set"); count != 2 {
		t.Fatalf("logged_set rows = %d after unknown exercise, want 2", count)
	}

	duplicate := app.LogSetsCommand{
		IdempotencyKey: "batch-4",
		SessionID:      session.ID,
		Sets: []domain.NewSet{
			{ExerciseID: "push-up", SetIndex: 2, KG: 0, Reps: 10},
			{ExerciseID: "push-up", SetIndex: 2, KG: 0, Reps: 8},
		},
	}
	_, err = service.LogSets(ctx, duplicate)
	requireTrainingCode(t, err, "set_position_occupied")
	if count := countTrainingRows(t, database, "logged_set"); count != 2 {
		t.Fatalf("logged_set rows = %d after duplicated position, want 2", count)
	}

	_, err = service.LogSets(ctx, app.LogSetsCommand{IdempotencyKey: "batch-5", SessionID: "missing", Sets: []domain.NewSet{{ExerciseID: "push-up", SetIndex: 1, KG: 0, Reps: 1}}})
	requireTrainingCode(t, err, "not_found")
}

func TestLogSetsUsesWatchEvidenceWhenLinked(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingExercise(t, database, "goblet-squat")
	seedTrainingWorkout(t, database, "watch-1", true, true)

	session := newTrainingSession(t, service, "session-1")
	if _, err := service.CheckIn(ctx, app.CheckInCommand{SessionID: session.ID, ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("watch-1")}); err != nil {
		t.Fatalf("check in watch: %v", err)
	}

	result, err := service.LogSets(ctx, app.LogSetsCommand{
		IdempotencyKey: "batch-1",
		SessionID:      session.ID,
		Sets:           []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 1, KG: 20, Reps: 10}},
	})
	if err != nil {
		t.Fatalf("log sets: %v", err)
	}
	if result.Sets[0].EvidenceSource != domain.EvidenceWatch {
		t.Fatalf("evidence source = %q, want watch", result.Sets[0].EvidenceSource)
	}

	if _, err := database.Exec(`UPDATE day_workout SET present = 0 WHERE id = 'watch-1'`); err != nil {
		t.Fatalf("remove workout: %v", err)
	}
	second, err := service.LogSets(ctx, app.LogSetsCommand{
		IdempotencyKey: "batch-2",
		SessionID:      session.ID,
		Sets:           []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 2, KG: 20, Reps: 10}},
	})
	if err != nil {
		t.Fatalf("log sets after workout removal: %v", err)
	}
	if second.Sets[0].EvidenceSource != domain.EvidenceReported {
		t.Fatalf("evidence source = %q, want reported", second.Sets[0].EvidenceSource)
	}
}

func TestLogSetsInvalidInput(t *testing.T) {
	service, database := newTrainingTestService(t)
	ctx := context.Background()
	seedTrainingExercise(t, database, "goblet-squat")
	session := newTrainingSession(t, service, "session-1")

	tests := []app.LogSetsCommand{
		{IdempotencyKey: "", SessionID: session.ID, Sets: []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 1, KG: 20, Reps: 10}}},
		{IdempotencyKey: "empty", SessionID: session.ID, Sets: nil},
		{IdempotencyKey: "position", SessionID: session.ID, Sets: []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 0, KG: 20, Reps: 10}}},
		{IdempotencyKey: "kg", SessionID: session.ID, Sets: []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 1, KG: -1, Reps: 10}}},
		{IdempotencyKey: "reps", SessionID: session.ID, Sets: []domain.NewSet{{ExerciseID: "goblet-squat", SetIndex: 1, KG: 20, Reps: 0}}},
	}

	for _, command := range tests {
		_, err := service.LogSets(ctx, command)
		if err == nil {
			t.Fatalf("command %+v succeeded, want an error", command)
		}
	}

	if count := countTrainingRows(t, database, "logged_set"); count != 0 {
		t.Fatalf("logged_set rows = %d, want 0", count)
	}
}

func TestSessionIndexesAndConstraints(t *testing.T) {
	_, database := newTrainingTestService(t)
	seedTrainingTemplate(t, database, "template-strength", domain.DisciplineStrength)

	if _, err := database.Exec(`INSERT INTO training_session (id, date, discipline, created_at, revision) VALUES ('bad', '2026-12-02', 'swim', 1, 1)`); err == nil {
		t.Fatal("unknown discipline was accepted")
	}
	if _, err := database.Exec(`INSERT INTO training_session (id, date, discipline, created_at, revision) VALUES ('bad', '2026-12-02', 'run', 1, 0)`); err == nil {
		t.Fatal("zero revision was accepted")
	}
	if _, err := database.Exec(`INSERT INTO logged_set (id, session_id, exercise_id, set_index, kg, reps, evidence_source, created_at, revision) VALUES ('s', 'missing', 'missing', 1, 0, 1, 'reported', 1, 1)`); err == nil {
		t.Fatal("logged set without a session was accepted")
	}
}
