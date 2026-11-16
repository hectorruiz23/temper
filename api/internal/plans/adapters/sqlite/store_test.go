package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/migrations"
)

func testStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "plans.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	return New(database), database
}

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func createExercise(t *testing.T, store *Store, id string, quantity int) {
	t.Helper()

	err := store.CreateExercise(context.Background(), domain.Exercise{
		ID:              id,
		Name:            id,
		MovementPattern: "squat",
		CreatedAt:       1000,
		Equipment:       []domain.ExerciseEquipment{{EquipmentID: "adjustable-dumbbell", Quantity: quantity}},
	})
	if err != nil {
		t.Fatalf("create exercise %s: %v", id, err)
	}
}

func validPlan(exerciseID string) domain.Plan {
	return domain.Plan{
		CreatedAt:       2000,
		Reason:          "base strength block",
		EffectiveFrom:   "2026-12-01",
		Timezone:        "Europe/Madrid",
		WeeklyFrequency: 3,
		GoalID:          "race-5k-2026-12-20",
		Sessions: []domain.Session{
			{
				ID:            "session-strength",
				Position:      0,
				ScheduledDate: stringPtr("2026-12-02"),
				Discipline:    domain.DisciplineStrength,
				Target:        "full body",
				Exercises: []domain.SessionExercise{
					{
						Position:   0,
						ExerciseID: exerciseID,
						TargetSets: intPtr(3),
						RepsMin:    intPtr(8),
						RepsMax:    intPtr(12),
						TargetKG:   floatPtr(20),
						Tempo:      stringPtr("3-1-1"),
					},
				},
			},
			{
				ID:         "session-intervals",
				Position:   1,
				Weekday:    intPtr(3),
				Discipline: domain.DisciplineIntervals,
				Target:     "5K pace",
				Steps: []domain.IntervalStep{
					{Position: 0, Label: "warm up", DurationMS: intPtr(600000)},
					{Position: 1, Label: "repetition", DistanceM: floatPtr(400)},
				},
			},
		},
	}
}

func requireCode(t *testing.T, err error, want string) {
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
}

func countRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestSeedCatalogue(t *testing.T) {
	store, database := testStore(t)

	inventory, err := store.Inventory(context.Background())
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(inventory) != 1 {
		t.Fatalf("inventory rows = %d, want 1: %+v", len(inventory), inventory)
	}
	item := inventory[0]
	if item.EquipmentID != "adjustable-dumbbell" || item.Name != "Adjustable dumbbell" || item.Quantity != 1 || item.MaxKG == nil || *item.MaxKG != 20 {
		t.Fatalf("seeded inventory = %+v", item)
	}

	if count := countRows(t, database, "equipment"); count != 1 {
		t.Fatalf("equipment rows = %d, want 1", count)
	}
	if count := countRows(t, database, "training_goal"); count != 1 {
		t.Fatalf("training_goal rows = %d, want 1", count)
	}
	for _, table := range []string{"exercise", "plan_version", "session_template", "interval_step", "session_exercise"} {
		if count := countRows(t, database, table); count != 0 {
			t.Fatalf("%s rows = %d, want 0", table, count)
		}
	}
}

func TestSchemaEnforcesForeignKeysAndChecks(t *testing.T) {
	_, database := testStore(t)

	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "foreign key on inventory equipment",
			query: `INSERT INTO inventory_item (equipment_id, quantity) VALUES ('ghost', 1)`,
		},
		{
			name:  "inventory quantity check",
			query: `INSERT INTO inventory_item (equipment_id, quantity) VALUES ('adjustable-dumbbell', 0)`,
		},
		{
			name:  "equipment unique name",
			query: `INSERT INTO equipment (id, name) VALUES ('other', 'Adjustable dumbbell')`,
		},
		{
			name:  "plan weekly frequency check",
			query: `INSERT INTO plan_version (reason, effective_from, timezone, weekly_frequency) VALUES ('x', '2026-01-01', 'UTC', 0)`,
		},
		{
			name:  "exercise equipment quantity check",
			query: `INSERT INTO exercise (id, name, movement_pattern) VALUES ('x', 'x', 'x'); INSERT INTO exercise_equipment (exercise_id, equipment_id, quantity) VALUES ('x', 'adjustable-dumbbell', 0)`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.Exec(test.query, test.args...); err == nil {
				t.Fatalf("statement succeeded, want a constraint failure")
			}
		})
	}

	var planID int64
	result, err := database.Exec(`INSERT INTO plan_version (reason, effective_from, timezone, weekly_frequency) VALUES ('x', '2026-01-01', 'UTC', 1)`)
	if err != nil {
		t.Fatalf("insert plan version: %v", err)
	}
	planID, err = result.LastInsertId()
	if err != nil {
		t.Fatalf("plan id: %v", err)
	}

	if _, err := database.Exec(`INSERT INTO session_template (id, plan_version_id, position, discipline) VALUES ('none', ?, 0, 'run')`, planID); err == nil {
		t.Fatalf("session with neither schedule field succeeded")
	}
	if _, err := database.Exec(`INSERT INTO session_template (id, plan_version_id, position, scheduled_date, weekday, discipline) VALUES ('both', ?, 0, '2026-01-02', 2, 'run')`, planID); err == nil {
		t.Fatalf("session with both schedule fields succeeded")
	}
	if _, err := database.Exec(`INSERT INTO session_template (id, plan_version_id, position, scheduled_date, discipline) VALUES ('ok', ?, 0, '2026-01-02', 'run')`, planID); err != nil {
		t.Fatalf("insert valid session: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO session_template (id, plan_version_id, position, scheduled_date, discipline) VALUES ('duplicate', ?, 0, '2026-01-03', 'run')`, planID); err == nil {
		t.Fatalf("duplicated session position succeeded")
	}
	if _, err := database.Exec(`INSERT INTO interval_step (template_id, position, label) VALUES ('ok', 0, 'neither')`); err == nil {
		t.Fatalf("interval step without measurement succeeded")
	}
	if _, err := database.Exec(`INSERT INTO interval_step (template_id, position, label, duration_ms, distance_m) VALUES ('ok', 1, 'both', 1000, 400)`); err == nil {
		t.Fatalf("interval step with both measurements succeeded")
	}
}

func TestCreateAndReadPlanGraph(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	createExercise(t, store, "goblet-squat", 1)

	if _, err := store.CurrentPlan(ctx); err == nil {
		t.Fatal("CurrentPlan succeeded on an empty database")
	} else {
		requireCode(t, err, "not_found")
	}

	plan := validPlan("goblet-squat")
	id, err := store.CreatePlan(ctx, plan)
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if id <= 0 {
		t.Fatalf("plan id = %d", id)
	}

	current, err := store.CurrentPlan(ctx)
	if err != nil {
		t.Fatalf("current plan: %v", err)
	}
	if current.ID != id || current.Reason != plan.Reason || current.EffectiveFrom != plan.EffectiveFrom || current.Timezone != plan.Timezone || current.WeeklyFrequency != plan.WeeklyFrequency || current.GoalID != plan.GoalID {
		t.Fatalf("current plan parent = %+v", current)
	}
	if len(current.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(current.Sessions))
	}
	if current.Sessions[0].ID != "session-strength" || current.Sessions[0].ScheduledDate == nil || *current.Sessions[0].ScheduledDate != "2026-12-02" {
		t.Fatalf("first session = %+v", current.Sessions[0])
	}
	if len(current.Sessions[0].Exercises) != 1 {
		t.Fatalf("session exercises = %d, want 1", len(current.Sessions[0].Exercises))
	}
	exercise := current.Sessions[0].Exercises[0]
	if exercise.ExerciseID != "goblet-squat" || exercise.TargetSets == nil || *exercise.TargetSets != 3 || exercise.RepsMin == nil || *exercise.RepsMin != 8 || exercise.RepsMax == nil || *exercise.RepsMax != 12 || exercise.TargetKG == nil || *exercise.TargetKG != 20 || exercise.Tempo == nil || *exercise.Tempo != "3-1-1" {
		t.Fatalf("session exercise = %+v", exercise)
	}
	if current.Sessions[1].Weekday == nil || *current.Sessions[1].Weekday != 3 || len(current.Sessions[1].Steps) != 2 {
		t.Fatalf("intervals session = %+v", current.Sessions[1])
	}
	if current.Sessions[1].Steps[0].DurationMS == nil || *current.Sessions[1].Steps[0].DurationMS != 600000 {
		t.Fatalf("first step = %+v", current.Sessions[1].Steps[0])
	}
	if current.Sessions[1].Steps[1].DistanceM == nil || *current.Sessions[1].Steps[1].DistanceM != 400 {
		t.Fatalf("second step = %+v", current.Sessions[1].Steps[1])
	}

	loaded, err := store.Plan(ctx, id)
	if err != nil {
		t.Fatalf("plan by id: %v", err)
	}
	if !reflect.DeepEqual(loaded, current) {
		t.Fatalf("Plan(id) and CurrentPlan differ:\n%+v\n%+v", loaded, current)
	}
	if _, err := store.Plan(ctx, id+100); err == nil {
		t.Fatal("Plan for a missing id succeeded")
	} else {
		requireCode(t, err, "not_found")
	}
}

func TestCreatePlanRollsBackEveryRow(t *testing.T) {
	store, database := testStore(t)
	ctx := context.Background()
	createExercise(t, store, "goblet-squat", 1)

	plan := validPlan("goblet-squat")
	duplicate := plan.Sessions[0]
	duplicate.Position = 2
	plan.Sessions = append(plan.Sessions, duplicate)

	if _, err := store.CreatePlan(ctx, plan); err == nil {
		t.Fatal("create plan succeeded with a duplicated session id")
	}

	for _, table := range []string{"plan_version", "session_template", "session_exercise", "interval_step"} {
		if count := countRows(t, database, table); count != 0 {
			t.Fatalf("%s rows = %d after rollback, want 0", table, count)
		}
	}
}

func TestCreatePlanRejections(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	createExercise(t, store, "goblet-squat", 1)
	createExercise(t, store, "double-dumbbell", 2)

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
				plan.Sessions[0].Exercises[0].TargetKG = floatPtr(21)
			},
		},
		{
			name: "unknown goal",
			code: "invalid_input",
			mutate: func(plan *domain.Plan) {
				plan.GoalID = "missing"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan("goblet-squat")
			test.mutate(&plan)
			_, err := store.CreatePlan(ctx, plan)
			requireCode(t, err, test.code)
		})
	}
}

func dumpVersionRows(t *testing.T, database *sql.DB, planID int64) map[string][]string {
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

func TestNewVersionLeavesEarlierVersionUntouched(t *testing.T) {
	store, database := testStore(t)
	ctx := context.Background()
	createExercise(t, store, "goblet-squat", 1)

	first := validPlan("goblet-squat")
	firstID, err := store.CreatePlan(ctx, first)
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}
	before := dumpVersionRows(t, database, firstID)

	second := validPlan("goblet-squat")
	second.CreatedAt = 3000
	second.Reason = "second block"
	second.EffectiveFrom = "2027-01-01"
	second.Sessions[0].ID = "session-second"
	second.Sessions[1].ID = "session-second-intervals"
	second.Sessions[0].Exercises[0].TargetKG = floatPtr(18)
	secondID, err := store.CreatePlan(ctx, second)
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("second plan reused id %d", firstID)
	}

	after := dumpVersionRows(t, database, firstID)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("version %d changed after creating version %d\nbefore: %+v\nafter: %+v", firstID, secondID, before, after)
	}
}

func TestIdempotentReplayAndConflict(t *testing.T) {
	store, database := testStore(t)
	ctx := context.Background()

	calls := 0
	mutate := func(ctx context.Context) ([]byte, error) {
		calls++
		if err := store.CreateExercise(ctx, domain.Exercise{ID: "goblet-squat", Name: "Goblet squat", MovementPattern: "squat", CreatedAt: 1000}); err != nil {
			return nil, err
		}
		return []byte(`{"id":"goblet-squat"}`), nil
	}

	first, err := store.Execute(ctx, "plans.create", "key-1", "hash-1", mutate)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	second, err := store.Execute(ctx, "plans.create", "key-1", "hash-1", mutate)
	if err != nil {
		t.Fatalf("replay execute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("mutation calls = %d, want 1", calls)
	}
	if string(first) != string(second) {
		t.Fatalf("replay result = %s, want %s", second, first)
	}
	if count := countRows(t, database, "exercise"); count != 1 {
		t.Fatalf("exercise rows = %d, want 1", count)
	}

	_, err = store.Execute(ctx, "plans.create", "key-1", "hash-2", mutate)
	requireCode(t, err, "idempotency_conflict")
	if calls != 1 {
		t.Fatalf("mutation calls after conflict = %d, want 1", calls)
	}

	var storedHash string
	var storedResult string
	if err := database.QueryRow(`SELECT request_hash, result_json FROM idempotency_request WHERE operation = ? AND key = ?`, "plans.create", "key-1").Scan(&storedHash, &storedResult); err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	if storedHash != "hash-1" || storedResult != `{"id":"goblet-squat"}` {
		t.Fatalf("receipt = %q %q", storedHash, storedResult)
	}
}

func TestCreateExerciseIsAppendOnly(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()

	createExercise(t, store, "goblet-squat", 1)
	err := store.CreateExercise(ctx, domain.Exercise{ID: "goblet-squat", Name: "Renamed", MovementPattern: "squat"})
	requireCode(t, err, "exercise_exists")

	exercises, err := store.ListExercises(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	if len(exercises) != 1 || exercises[0].Name != "goblet-squat" || len(exercises[0].Equipment) != 1 {
		t.Fatalf("exercises = %+v", exercises)
	}
}
