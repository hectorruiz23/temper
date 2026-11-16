package acceptance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/plans/adapters/sqlite"
	"github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/migrations"
)

type planClock struct {
	at time.Time
}

func (c planClock) Now() time.Time {
	return c.at
}

type planFixture struct {
	database  *sql.DB
	store     *sqlite.Store
	plans     *app.PlanService
	catalogue *app.CatalogueService
	clock     planClock
}

func newPlanFixture(t *testing.T) *planFixture {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "plans.db"))
	if err != nil {
		t.Fatalf("open plans database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate plans database: %v", err)
	}

	clock := planClock{at: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	store := sqlite.New(database)

	return &planFixture{
		database:  database,
		store:     store,
		plans:     app.NewPlanService(store, store, clock),
		catalogue: app.NewCatalogueService(store, clock),
		clock:     clock,
	}
}

func planIntPtr(value int) *int {
	return &value
}

func planFloatPtr(value float64) *float64 {
	return &value
}

func planStringPtr(value string) *string {
	return &value
}

func (f *planFixture) createExercise(t *testing.T, id string, quantity int) domain.Exercise {
	t.Helper()

	exercise, err := f.catalogue.CreateExercise(context.Background(), app.CreateExerciseCommand{
		ID:              id,
		Name:            id,
		MovementPattern: "squat",
		Equipment:       []domain.ExerciseEquipment{{EquipmentID: "adjustable-dumbbell", Quantity: quantity}},
	})
	if err != nil {
		t.Fatalf("create exercise %s: %v", id, err)
	}
	return exercise
}

func acceptancePlan(exerciseID, suffix string) domain.Plan {
	return domain.Plan{
		Reason:          "base strength block",
		EffectiveFrom:   "2026-12-01",
		Timezone:        "Europe/Madrid",
		WeeklyFrequency: 3,
		GoalID:          "race-5k-2026-12-20",
		Sessions: []domain.Session{
			{
				ID:            "session-strength-" + suffix,
				Position:      0,
				ScheduledDate: planStringPtr("2026-12-02"),
				Discipline:    domain.DisciplineStrength,
				Target:        "full body",
				Exercises: []domain.SessionExercise{
					{
						Position:   0,
						ExerciseID: exerciseID,
						TargetSets: planIntPtr(3),
						RepsMin:    planIntPtr(8),
						RepsMax:    planIntPtr(12),
						TargetKG:   planFloatPtr(20),
						Tempo:      planStringPtr("3-1-1"),
					},
				},
			},
			{
				ID:         "session-intervals-" + suffix,
				Position:   1,
				Weekday:    planIntPtr(3),
				Discipline: domain.DisciplineIntervals,
				Target:     "5K pace",
				Steps: []domain.IntervalStep{
					{Position: 0, Label: "warm up", DurationMS: planIntPtr(600000)},
					{Position: 1, Label: "repetition", DistanceM: planFloatPtr(400)},
				},
			},
		},
	}
}

func requirePlanCode(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want code %q", want)
	}
	var ruleError kernel.RuleError
	if !errors.As(err, &ruleError) {
		t.Fatalf("error %v is not a kernel.RuleError", err)
	}
	if ruleError.Code != want {
		t.Fatalf("code = %q, want %q (message %q)", ruleError.Code, want, ruleError.Message)
	}
	if ruleError.Message == "" {
		t.Fatalf("code %q has an empty message", want)
	}
}

func planCount(t *testing.T, database *sql.DB) int {
	t.Helper()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM plan_version`).Scan(&count); err != nil {
		t.Fatalf("count plan versions: %v", err)
	}
	return count
}

func TestPlanServicesCreateAndReadCurrent(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)

	if _, err := fixture.plans.GetCurrentPlan(ctx); err == nil {
		t.Fatal("GetCurrentPlan succeeded before any version exists")
	} else {
		requirePlanCode(t, err, "not_found")
	}

	created, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{
		IdempotencyKey: "plan-v1",
		Plan:           acceptancePlan("goblet-squat", "v1"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("created plan id = %d", created.ID)
	}
	if created.CreatedAt != fixture.clock.at.UnixMilli() {
		t.Fatalf("created_at = %d, want %d", created.CreatedAt, fixture.clock.at.UnixMilli())
	}
	if len(created.Sessions) != 2 || len(created.Sessions[0].Exercises) != 1 || len(created.Sessions[1].Steps) != 2 {
		t.Fatalf("created graph is incomplete: %+v", created)
	}

	current, err := fixture.plans.GetCurrentPlan(ctx)
	if err != nil {
		t.Fatalf("GetCurrentPlan: %v", err)
	}
	if !reflect.DeepEqual(current, created) {
		t.Fatalf("GetCurrentPlan = %+v, want %+v", current, created)
	}

	loaded, err := fixture.plans.GetPlan(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if !reflect.DeepEqual(loaded, created) {
		t.Fatalf("GetPlan = %+v, want %+v", loaded, created)
	}

	if _, err := fixture.plans.GetPlan(ctx, created.ID+1000); err == nil {
		t.Fatal("GetPlan succeeded for a missing version")
	} else {
		requirePlanCode(t, err, "not_found")
	}

	secondCommand := app.CreatePlanCommand{
		IdempotencyKey: "plan-v2",
		Plan:           acceptancePlan("goblet-squat", "v2"),
	}
	secondCommand.Plan.EffectiveFrom = "2027-01-01"
	second, err := fixture.plans.CreatePlan(ctx, secondCommand)
	if err != nil {
		t.Fatalf("CreatePlan second version: %v", err)
	}

	current, err = fixture.plans.GetCurrentPlan(ctx)
	if err != nil {
		t.Fatalf("GetCurrentPlan after second version: %v", err)
	}
	if current.ID != second.ID {
		t.Fatalf("current version = %d, want %d", current.ID, second.ID)
	}
}

func TestPlanServiceRejections(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)
	fixture.createExercise(t, "double-dumbbell", 2)

	tests := []struct {
		name   string
		code   string
		mutate func(*domain.Plan)
	}{
		{
			name: "unknown exercise",
			code: "unknown_exercise",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Exercises[0].ExerciseID = "missing"
			},
		},
		{
			name: "unavailable equipment",
			code: "unavailable_equipment",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Exercises[0].ExerciseID = "double-dumbbell"
			},
		},
		{
			name: "equipment limit",
			code: "equipment_limit",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Exercises[0].TargetKG = planFloatPtr(21)
			},
		},
		{
			name: "invalid schedule",
			code: "invalid_schedule",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Weekday = planIntPtr(2)
			},
		},
		{
			name: "invalid target",
			code: "invalid_target",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Exercises[0].RepsMin = planIntPtr(12)
				plan.Sessions[0].Exercises[0].RepsMax = planIntPtr(8)
			},
		},
		{
			name: "invalid interval steps",
			code: "invalid_interval_steps",
			mutate: func(plan *domain.Plan) {
				plan.Sessions[0].Discipline = domain.DisciplineIntervals
				plan.Sessions[0].Exercises = nil
			},
		},
		{
			name: "invalid input",
			code: "invalid_input",
			mutate: func(plan *domain.Plan) {
				plan.Reason = " "
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := acceptancePlan("goblet-squat", test.name)
			test.mutate(&plan)
			_, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{
				IdempotencyKey: "reject-" + test.name,
				Plan:           plan,
			})
			requirePlanCode(t, err, test.code)
			if count := planCount(t, fixture.database); count != 0 {
				t.Fatalf("plan_version rows = %d after rejection, want 0", count)
			}
		})
	}
}

func TestPlanServiceIdempotentReplayAndConflict(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)

	command := app.CreatePlanCommand{
		IdempotencyKey: "replay-key",
		Plan:           acceptancePlan("goblet-squat", "replay"),
	}

	first, err := fixture.plans.CreatePlan(ctx, command)
	if err != nil {
		t.Fatalf("first CreatePlan: %v", err)
	}
	second, err := fixture.plans.CreatePlan(ctx, command)
	if err != nil {
		t.Fatalf("replayed CreatePlan: %v", err)
	}
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("replay = %+v, want %+v", second, first)
	}
	if count := planCount(t, fixture.database); count != 1 {
		t.Fatalf("plan_version rows = %d after replay, want 1", count)
	}

	changed := command
	changed.Plan.Reason = "different body"
	_, err = fixture.plans.CreatePlan(ctx, changed)
	requirePlanCode(t, err, "idempotency_conflict")
	if count := planCount(t, fixture.database); count != 1 {
		t.Fatalf("plan_version rows = %d after conflict, want 1", count)
	}
}

func dumpPlanRows(t *testing.T, database *sql.DB, planID int64) map[string][]string {
	t.Helper()

	queries := map[string]string{
		"plan_version":     `SELECT * FROM plan_version WHERE id = ?`,
		"session_template": `SELECT * FROM session_template WHERE plan_version_id = ? ORDER BY position, id`,
		"session_exercise": `SELECT se.* FROM session_exercise se JOIN session_template st ON st.id = se.template_id WHERE st.plan_version_id = ? ORDER BY se.template_id, se.position`,
		"interval_step":    `SELECT istep.* FROM interval_step istep JOIN session_template st ON st.id = istep.template_id WHERE st.plan_version_id = ? ORDER BY istep.template_id, istep.position`,
	}

	result := make(map[string][]string, len(queries))
	for table, query := range queries {
		rows, err := database.Query(query, planID)
		if err != nil {
			t.Fatalf("dump %s: %v", table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("columns of %s: %v", table, err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			line := ""
			for index, value := range values {
				line += fmt.Sprintf("%s=%T:%v|", columns[index], value, value)
			}
			result[table] = append(result[table], line)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate %s: %v", table, err)
		}
		rows.Close()
	}

	return result
}

func TestEarlierPlanVersionIsUnchangedAfterNewVersion(t *testing.T) {
	fixture := newPlanFixture(t)
	ctx := context.Background()
	fixture.createExercise(t, "goblet-squat", 1)

	first, err := fixture.plans.CreatePlan(ctx, app.CreatePlanCommand{
		IdempotencyKey: "immutable-v1",
		Plan:           acceptancePlan("goblet-squat", "immutable-v1"),
	})
	if err != nil {
		t.Fatalf("CreatePlan first version: %v", err)
	}
	before := dumpPlanRows(t, fixture.database, first.ID)

	secondCommand := app.CreatePlanCommand{
		IdempotencyKey: "immutable-v2",
		Plan:           acceptancePlan("goblet-squat", "immutable-v2"),
	}
	secondCommand.Plan.EffectiveFrom = "2027-01-01"
	secondCommand.Plan.Reason = "second block"
	secondCommand.Plan.Sessions[0].Exercises[0].TargetKG = planFloatPtr(18)
	second, err := fixture.plans.CreatePlan(ctx, secondCommand)
	if err != nil {
		t.Fatalf("CreatePlan second version: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("second version reused id %d", first.ID)
	}

	after := dumpPlanRows(t, fixture.database, first.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("version %d changed after creating version %d\nbefore: %+v\nafter: %+v", first.ID, second.ID, before, after)
	}

	current, err := fixture.plans.GetCurrentPlan(ctx)
	if err != nil {
		t.Fatalf("GetCurrentPlan: %v", err)
	}
	if current.ID != second.ID {
		t.Fatalf("current version = %d, want %d", current.ID, second.ID)
	}
}
