package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hectorruiz23/temper/api/internal/health/app"
	"github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/migrations"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func intPtr(value int64) *int64 {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func newTestService(t *testing.T) (*app.Service, *Store, *sql.DB) {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	store := New(database)
	service := app.New(store, fixedClock{now: time.Unix(1763300000, 0)})
	return service, store, database
}

func createCommand(key string) app.UpsertDayCommand {
	return app.UpsertDayCommand{
		Date:             "2026-11-16",
		Timezone:         "Europe/Madrid",
		ExpectedRevision: 0,
		Source:           domain.SourceManual,
		Metrics: domain.MetricsPatch{
			SleepMS:      domain.OptionalInt64{Set: true, Value: intPtr(25200000)},
			Steps:        domain.OptionalInt64{Set: true, Value: intPtr(0)},
			RestingHRBPM: domain.OptionalInt64{Set: true, Value: intPtr(52)},
			HRVRMSSDMS:   domain.OptionalFloat64{Set: true, Value: floatPtr(41.5)},
		},
		SleepStages: &[]domain.SleepStage{{Stage: "deep", DurationMS: 7200000}},
		Workouts: &[]domain.Workout{{
			Origin:        "health_connect",
			ExternalID:    "abc-123",
			DeviceKind:    stringPtr("watch"),
			StartedAt:     1763300000000,
			EndedAt:       1763303600000,
			Discipline:    "run",
			DurationMS:    3600000,
			DistanceM:     floatPtr(5200),
			WatchVerified: true,
		}},
		IdempotencyKey: key,
	}
}

func TestUpsertDayIsIdempotent(t *testing.T) {
	service, _, database := newTestService(t)
	ctx := context.Background()
	command := createCommand("key-1")

	first, err := service.UpsertDay(ctx, command)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.Revision != 1 || first.SleepMS == nil || *first.SleepMS != 25200000 {
		t.Fatalf("first day = %+v", first)
	}
	if first.Steps == nil || *first.Steps != 0 {
		t.Fatalf("steps = %v, want explicit zero", first.Steps)
	}

	second, err := service.UpsertDay(ctx, command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Revision != first.Revision || second.UpdatedAt != first.UpdatedAt {
		t.Fatalf("replay changed the day: %+v then %+v", first, second)
	}
	if len(second.Workouts) != len(first.Workouts) || second.Workouts[0].ID != first.Workouts[0].ID {
		t.Fatalf("replay changed workouts: %+v then %+v", first.Workouts, second.Workouts)
	}

	differentKey := command
	differentKey.ExpectedRevision = 1
	differentKey.IdempotencyKey = "key-2"
	third, err := service.UpsertDay(ctx, differentKey)
	if err != nil {
		t.Fatalf("same body with a new key: %v", err)
	}
	if third.Revision != 2 {
		t.Fatalf("revision after a successful mutation = %d, want 2", third.Revision)
	}

	var dayCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM day`).Scan(&dayCount); err != nil {
		t.Fatalf("count days: %v", err)
	}
	if dayCount != 1 {
		t.Fatalf("day rows = %d, want 1", dayCount)
	}
	var receiptCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM idempotency_request`).Scan(&receiptCount); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if receiptCount != 2 {
		t.Fatalf("receipts = %d, want 2", receiptCount)
	}
}

func TestTwoDifferentSubmitsOnSameDate(t *testing.T) {
	service, _, database := newTestService(t)
	ctx := context.Background()

	if _, err := service.UpsertDay(ctx, createCommand("key-1")); err != nil {
		t.Fatalf("create: %v", err)
	}

	update := app.UpsertDayCommand{
		Date:             "2026-11-16",
		Timezone:         "Europe/Madrid",
		ExpectedRevision: 1,
		Source:           domain.SourceHealthConnect,
		Metrics: domain.MetricsPatch{
			Steps: domain.OptionalInt64{Set: true, Value: intPtr(20)},
		},
		IdempotencyKey: "key-2",
	}
	updated, err := service.UpsertDay(ctx, update)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Revision != 2 || updated.Steps == nil || *updated.Steps != 20 {
		t.Fatalf("updated day = %+v", updated)
	}
	if updated.SleepMS == nil || *updated.SleepMS != 25200000 {
		t.Fatalf("omitted metric was not preserved: %v", updated.SleepMS)
	}
	if updated.LastSource != domain.SourceHealthConnect {
		t.Fatalf("last_source = %q", updated.LastSource)
	}

	stale := update
	stale.IdempotencyKey = "key-3"
	if _, err := service.UpsertDay(ctx, stale); err == nil {
		t.Fatal("stale update succeeded")
	} else {
		var rule kernel.RuleError
		if !errors.As(err, &rule) || rule.Code != "revision_conflict" {
			t.Fatalf("stale update error = %v", err)
		}
	}

	var revision int64
	if err := database.QueryRowContext(ctx, `SELECT revision FROM day WHERE date = ?`, update.Date).Scan(&revision); err != nil {
		t.Fatalf("read revision: %v", err)
	}
	if revision != 2 {
		t.Fatalf("revision after a rejected update = %d, want 2", revision)
	}
}

func TestStepsSourceAndStepsAreReplacedOnResync(t *testing.T) {
	service, _, database := newTestService(t)
	ctx := context.Background()

	first := createCommand("key-1")
	first.Metrics.Steps = domain.OptionalInt64{Set: true, Value: intPtr(24072)}
	first.StepsSource = stringPtr("phone")
	if _, err := service.UpsertDay(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := createCommand("key-2")
	second.ExpectedRevision = 1
	second.Source = domain.SourceHealthConnect
	second.Metrics.Steps = domain.OptionalInt64{Set: true, Value: intPtr(11000)}
	second.StepsSource = stringPtr("watch")
	resynced, err := service.UpsertDay(ctx, second)
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if resynced.Steps == nil || *resynced.Steps != 11000 {
		t.Fatalf("steps after resync = %v, want 11000", resynced.Steps)
	}
	if resynced.StepsSource == nil || *resynced.StepsSource != "watch" {
		t.Fatalf("steps_source after resync = %v, want watch", resynced.StepsSource)
	}
	if resynced.Revision != 2 {
		t.Fatalf("revision after resync = %d, want 2", resynced.Revision)
	}

	third := createCommand("key-3")
	third.ExpectedRevision = 2
	third.Metrics.Steps = domain.OptionalInt64{Set: true, Value: intPtr(11000)}
	third.StepsSource = stringPtr("watch")
	unchanged, err := service.UpsertDay(ctx, third)
	if err != nil {
		t.Fatalf("second resync: %v", err)
	}
	if unchanged.Steps == nil || *unchanged.Steps != 11000 {
		t.Fatalf("steps after the second resync = %v, want 11000", unchanged.Steps)
	}

	fourth := createCommand("key-4")
	fourth.ExpectedRevision = 3
	fourth.Date = first.Date
	fourth.StepsSource = nil
	fourth.Metrics.Steps = domain.OptionalInt64{}
	fourth.Metrics.SleepMS = domain.OptionalInt64{Set: true, Value: intPtr(26000000)}
	partial, err := service.UpsertDay(ctx, fourth)
	if err != nil {
		t.Fatalf("partial upsert: %v", err)
	}
	if partial.StepsSource == nil || *partial.StepsSource != "watch" {
		t.Fatalf("steps_source after a partial upsert = %v, want watch", partial.StepsSource)
	}
	if partial.Steps == nil || *partial.Steps != 11000 {
		t.Fatalf("steps after a partial upsert = %d, want 11000", *partial.Steps)
	}

	var steps int64
	var source sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT steps, steps_source FROM day WHERE date = ?`, first.Date).Scan(&steps, &source); err != nil {
		t.Fatalf("read stored day: %v", err)
	}
	if steps != 11000 || !source.Valid || source.String != "watch" {
		t.Fatalf("stored steps and steps_source = %d/%v, want 11000/watch", steps, source)
	}
}

func TestMetricsOmittedVersusExplicitNull(t *testing.T) {
	service, _, _ := newTestService(t)
	ctx := context.Background()
	command := createCommand("key-1")
	command.Metrics.Steps = domain.OptionalInt64{Set: true, Value: intPtr(5)}

	if _, err := service.UpsertDay(ctx, command); err != nil {
		t.Fatalf("create: %v", err)
	}

	omitted := app.UpsertDayCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: 1,
		Source:           domain.SourceManual,
		IdempotencyKey:   "key-2",
	}
	unchanged, err := service.UpsertDay(ctx, omitted)
	if err != nil {
		t.Fatalf("omitted metrics: %v", err)
	}
	if unchanged.SleepMS == nil || *unchanged.SleepMS != 25200000 || unchanged.HRVRMSSDMS == nil || *unchanged.HRVRMSSDMS != 41.5 {
		t.Fatalf("omitted metrics changed: %+v", unchanged)
	}
	if unchanged.Steps == nil || *unchanged.Steps != 5 {
		t.Fatalf("omitted steps = %v, want 5", unchanged.Steps)
	}

	cleared := app.UpsertDayCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: 2,
		Source:           domain.SourceHealthConnect,
		Metrics: domain.MetricsPatch{
			SleepMS:    domain.OptionalInt64{Set: true},
			HRVRMSSDMS: domain.OptionalFloat64{Set: true},
			Steps:      domain.OptionalInt64{Set: true, Value: intPtr(0)},
		},
		IdempotencyKey: "key-3",
	}
	nulled, err := service.UpsertDay(ctx, cleared)
	if err != nil {
		t.Fatalf("null metrics: %v", err)
	}
	if nulled.SleepMS != nil || nulled.HRVRMSSDMS != nil {
		t.Fatalf("explicit null did not clear: %+v", nulled)
	}
	if nulled.Steps == nil || *nulled.Steps != 0 {
		t.Fatalf("explicit zero steps = %v, want 0", nulled.Steps)
	}
	if nulled.RestingHRBPM == nil || *nulled.RestingHRBPM != 52 {
		t.Fatalf("untouched heart rate = %v, want 52", nulled.RestingHRBPM)
	}
}

func TestWorkoutListReplacementKeepsRowsAndIDs(t *testing.T) {
	service, _, database := newTestService(t)
	ctx := context.Background()

	command := createCommand("key-1")
	command.Workouts = &[]domain.Workout{
		{Origin: "health_connect", ExternalID: "a", StartedAt: 1, EndedAt: 2, Discipline: "run"},
		{Origin: "health_connect", ExternalID: "b", StartedAt: 3, EndedAt: 4, Discipline: "ride"},
	}
	created, err := service.UpsertDay(ctx, command)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Workouts) != 2 {
		t.Fatalf("created workouts = %+v", created.Workouts)
	}
	ids := make(map[string]string, 2)
	for _, workout := range created.Workouts {
		ids[workout.ExternalID] = workout.ID
	}

	withoutB := app.UpsertDayCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: 1,
		Source:           domain.SourceHealthConnect,
		Workouts: &[]domain.Workout{
			{Origin: "health_connect", ExternalID: "a", StartedAt: 10, EndedAt: 20, Discipline: "run", DistanceM: floatPtr(100)},
		},
		IdempotencyKey: "key-2",
	}
	replaced, err := service.UpsertDay(ctx, withoutB)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	surviving := presentWorkouts(replaced.Workouts)
	if len(surviving) != 1 || surviving[0].ExternalID != "a" || surviving[0].ID != ids["a"] {
		t.Fatalf("replaced workouts = %+v", surviving)
	}

	var rows int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM day_workout WHERE day_date = ?`, command.Date).Scan(&rows); err != nil {
		t.Fatalf("count workouts: %v", err)
	}
	if rows != 2 {
		t.Fatalf("workout rows = %d, want 2", rows)
	}
	var present int
	if err := database.QueryRowContext(ctx, `SELECT present FROM day_workout WHERE id = ?`, ids["b"]).Scan(&present); err != nil {
		t.Fatalf("read present: %v", err)
	}
	if present != 0 {
		t.Fatalf("removed workout present = %d, want 0", present)
	}

	withBBoth := app.UpsertDayCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: 2,
		Source:           domain.SourceHealthConnect,
		Workouts: &[]domain.Workout{
			{Origin: "health_connect", ExternalID: "b", StartedAt: 30, EndedAt: 40, Discipline: "ride"},
		},
		IdempotencyKey: "key-3",
	}
	reappeared, err := service.UpsertDay(ctx, withBBoth)
	if err != nil {
		t.Fatalf("reappear: %v", err)
	}
	returned := presentWorkouts(reappeared.Workouts)
	if len(returned) != 1 || returned[0].ID != ids["b"] {
		t.Fatalf("reappeared workout = %+v", returned)
	}

	empty := app.UpsertDayCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: 3,
		Source:           domain.SourceHealthConnect,
		Workouts:         &[]domain.Workout{},
		IdempotencyKey:   "key-4",
	}
	emptied, err := service.UpsertDay(ctx, empty)
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if len(presentWorkouts(emptied.Workouts)) != 0 {
		t.Fatalf("emptied workouts = %+v", emptied.Workouts)
	}

	var remaining int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM day_workout WHERE day_date = ? AND present = 1`, command.Date).Scan(&remaining); err != nil {
		t.Fatalf("count present: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("present workouts = %d, want 0", remaining)
	}
}

func TestUniqueOriginAndExternalID(t *testing.T) {
	_, store, _ := newTestService(t)
	ctx := context.Background()

	day := domain.Day{
		Date:       "2026-11-16",
		Timezone:   "Europe/Madrid",
		Revision:   1,
		UpdatedAt:  1,
		LastSource: domain.SourceManual,
		Workouts: []domain.Workout{
			{ID: "one", Origin: "health_connect", ExternalID: "same", StartedAt: 1, EndedAt: 2, Discipline: "run", Present: true},
			{ID: "two", Origin: "health_connect", ExternalID: "same", StartedAt: 3, EndedAt: 4, Discipline: "run", Present: true},
		},
	}

	err := store.WithinTransaction(ctx, func(ctx context.Context, days app.DayStore, idempotency app.IdempotencyStore) error {
		return days.SaveDay(ctx, day)
	})
	if err == nil {
		t.Fatal("duplicate origin/external_id was accepted")
	}

	if _, err := store.GetDay(ctx, day.Date); !errors.Is(err, domain.ErrDayNotFound) {
		t.Fatalf("day survived a rolled back transaction: %v", err)
	}
}

func TestRollbackWhenChildRowIsInvalid(t *testing.T) {
	_, store, _ := newTestService(t)
	ctx := context.Background()

	day := domain.Day{
		Date:        "2026-11-16",
		Timezone:    "Europe/Madrid",
		Revision:    1,
		UpdatedAt:   1,
		LastSource:  domain.SourceManual,
		SleepStages: []domain.SleepStage{{Stage: "deep", DurationMS: -1}},
	}

	err := store.WithinTransaction(ctx, func(ctx context.Context, days app.DayStore, idempotency app.IdempotencyStore) error {
		return days.SaveDay(ctx, day)
	})
	if err == nil {
		t.Fatal("invalid sleep stage was accepted")
	}

	if _, err := store.GetDay(ctx, day.Date); !errors.Is(err, domain.ErrDayNotFound) {
		t.Fatalf("day survived a rolled back child write: %v", err)
	}
}

func TestGetDayMissing(t *testing.T) {
	_, store, _ := newTestService(t)

	if _, err := store.GetDay(context.Background(), "2026-11-16"); !errors.Is(err, domain.ErrDayNotFound) {
		t.Fatalf("GetDay error = %v, want ErrDayNotFound", err)
	}
}

func presentWorkouts(workouts []domain.Workout) []domain.Workout {
	present := make([]domain.Workout, 0, len(workouts))
	for _, workout := range workouts {
		if workout.Present {
			present = append(present, workout)
		}
	}
	return present
}
