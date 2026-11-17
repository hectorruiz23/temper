package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	healthsqlite "github.com/hectorruiz23/temper/api/internal/health/adapters/sqlite"
	healthapp "github.com/hectorruiz23/temper/api/internal/health/app"
	healthdomain "github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	planssqlite "github.com/hectorruiz23/temper/api/internal/plans/adapters/sqlite"
	"github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	trainingsqlite "github.com/hectorruiz23/temper/api/internal/training/adapters/sqlite"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
	trainingdomain "github.com/hectorruiz23/temper/api/internal/training/domain"
	"github.com/hectorruiz23/temper/api/migrations"
)

const handlerToken = "handler-test-token"

type fixedClock struct {
	at time.Time
}

func (c fixedClock) Now() time.Time {
	return c.at
}

type testFixture struct {
	handler   *httpserver.Router
	database  *sql.DB
	plans     *app.PlanService
	catalogue *app.CatalogueService
	health    *healthapp.Service
	training  *trainingapp.Service
	clock     fixedClock
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "plans-http.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	clock := fixedClock{at: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	planStore := planssqlite.New(database)
	healthStore := healthsqlite.New(database)
	trainingStore := trainingsqlite.New(database)

	fixture := &testFixture{
		database:  database,
		plans:     app.NewPlanService(planStore, planStore, clock),
		catalogue: app.NewCatalogueService(planStore, clock),
		health:    healthapp.New(healthStore, clock),
		training:  trainingapp.New(trainingStore, clock),
		clock:     clock,
	}

	router := httpserver.NewRouter(handlerToken)
	planHandler := NewPlanHandler(fixture.plans)
	catalogueHandler := NewCatalogueHandler(fixture.catalogue)
	weekHandler := NewWeekHandler(fixture.plans, fixture.health, fixture.training)

	router.HandleFunc("POST /api/plans", planHandler.Create)
	router.HandleFunc("GET /api/plans/current", planHandler.Current)
	router.HandleFunc("GET /api/plans/{id}", planHandler.Get)
	router.HandleFunc("GET /api/week/{date}", weekHandler.Get)
	router.HandleFunc("GET /api/inventory", catalogueHandler.Inventory)
	router.HandleFunc("GET /api/exercises", catalogueHandler.List)
	router.HandleFunc("POST /api/exercises", catalogueHandler.Create)
	fixture.handler = router

	return fixture
}

func (f *testFixture) createExercise(t *testing.T, id string, quantity int) {
	t.Helper()

	if _, err := f.catalogue.CreateExercise(context.Background(), app.CreateExerciseCommand{
		ID:              id,
		Name:            id,
		MovementPattern: "squat",
		Equipment:       []domain.ExerciseEquipment{{EquipmentID: "adjustable-dumbbell", Quantity: quantity}},
	}); err != nil {
		t.Fatalf("create exercise %s: %v", id, err)
	}
}

func (f *testFixture) createDay(t *testing.T, date, key string, sleepMS, steps, restingHRBPM int64) {
	t.Helper()

	sleep := sleepMS
	stepCount := steps
	restingHR := restingHRBPM

	if _, err := f.health.UpsertDay(context.Background(), healthapp.UpsertDayCommand{
		Date:           date,
		Timezone:       "Europe/Madrid",
		Source:         healthdomain.SourceManual,
		IdempotencyKey: key,
		Metrics: healthdomain.MetricsPatch{
			SleepMS:      healthdomain.OptionalInt64{Set: true, Value: &sleep},
			Steps:        healthdomain.OptionalInt64{Set: true, Value: &stepCount},
			RestingHRBPM: healthdomain.OptionalInt64{Set: true, Value: &restingHR},
		},
	}); err != nil {
		t.Fatalf("create day %s: %v", date, err)
	}
}

func callHandler(t *testing.T, handler *httpserver.Router, method, path, token, key, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if token != "" {
		request.Header.Set("X-Temper-Token", token)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requireError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}

	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error body %q: %v", recorder.Body.String(), err)
	}
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q; body = %s", response.Error.Code, code, recorder.Body.String())
	}
	if response.Error.Message == "" {
		t.Fatalf("error message is empty; body = %s", recorder.Body.String())
	}
}

func decodePlan(t *testing.T, recorder *httptest.ResponseRecorder) planResponse {
	t.Helper()

	if recorder.Code != 201 && recorder.Code != 200 {
		t.Fatalf("unexpected status %d; body = %s", recorder.Code, recorder.Body.String())
	}

	var envelope planEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode plan body %q: %v", recorder.Body.String(), err)
	}
	return envelope.Plan
}

func planBody(exerciseID string) string {
	return fmt.Sprintf(`{
		"reason":"base strength block",
		"effective_from":"2026-12-01",
		"timezone":"Europe/Madrid",
		"weekly_frequency":3,
		"sessions":[
			{
				"id":"session-strength",
				"position":0,
				"scheduled_date":"2026-12-02",
				"discipline":"strength",
				"target":"full body",
				"exercises":[
					{"position":0,"exercise_id":%q,"target_sets":3,"reps_min":8,"reps_max":12,"target_kg":20,"tempo":"3-1-1"}
				]
			},
			{
				"id":"session-intervals",
				"position":1,
				"weekday":3,
				"discipline":"intervals",
				"target":"5K pace",
				"interval_steps":[
					{"position":0,"label":"warm up","duration_ms":600000},
					{"position":1,"label":"repetition","distance_m":400}
				]
			}
		]
	}`, exerciseID)
}

func TestPlanCreateReadReplayAndConflict(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.createExercise(t, "goblet-squat", 1)
	body := planBody("goblet-squat")

	requireError(t, callHandler(t, fixture.handler, "GET", "/api/plans/current", handlerToken, "", ""), 404, "not_found")

	createdRecorder := callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "plan-1", body)
	created := decodePlan(t, createdRecorder)
	if createdRecorder.Code != 201 {
		t.Fatalf("create status = %d, want 201", createdRecorder.Code)
	}
	if created.ID <= 0 {
		t.Fatalf("created id = %d", created.ID)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt); err != nil {
		t.Fatalf("created_at %q is not RFC 3339: %v", created.CreatedAt, err)
	}
	if created.Reason != "base strength block" || created.EffectiveFrom != "2026-12-01" || created.Timezone != "Europe/Madrid" || created.WeeklyFrequency != 3 {
		t.Fatalf("created plan parent = %+v", created)
	}
	if len(created.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(created.Sessions))
	}
	strength := created.Sessions[0]
	if strength.ID != "session-strength" || strength.ScheduledDate == nil || *strength.ScheduledDate != "2026-12-02" || strength.Weekday != nil {
		t.Fatalf("strength session = %+v", strength)
	}
	if len(strength.Exercises) != 1 || len(strength.IntervalSteps) != 0 {
		t.Fatalf("strength children = %+v", strength)
	}
	exercise := strength.Exercises[0]
	if exercise.ExerciseID != "goblet-squat" || exercise.TargetSets == nil || *exercise.TargetSets != 3 || exercise.TargetKG == nil || *exercise.TargetKG != 20 || exercise.Tempo == nil || *exercise.Tempo != "3-1-1" {
		t.Fatalf("strength exercise = %+v", exercise)
	}
	intervals := created.Sessions[1]
	if intervals.ID != "session-intervals" || intervals.Weekday == nil || *intervals.Weekday != 3 || intervals.ScheduledDate != nil {
		t.Fatalf("intervals session = %+v", intervals)
	}
	if len(intervals.IntervalSteps) != 2 || len(intervals.Exercises) != 0 {
		t.Fatalf("interval children = %+v", intervals)
	}
	if intervals.IntervalSteps[0].DurationMS == nil || *intervals.IntervalSteps[0].DurationMS != 600000 {
		t.Fatalf("first interval step = %+v", intervals.IntervalSteps[0])
	}
	if !strings.Contains(createdRecorder.Body.String(), `"interval_steps"`) || strings.Contains(createdRecorder.Body.String(), `"steps"`) {
		t.Fatalf("plan body does not use interval_steps only: %s", createdRecorder.Body.String())
	}

	replayed := decodePlan(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "plan-1", body))
	if replayed.ID != created.ID || replayed.CreatedAt != created.CreatedAt {
		t.Fatalf("replay = %+v, want %+v", replayed, created)
	}

	conflicting := strings.Replace(body, `"reason":"base strength block"`, `"reason":"different block"`, 1)
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "plan-1", conflicting), 409, "idempotency_conflict")

	current := decodePlan(t, callHandler(t, fixture.handler, "GET", "/api/plans/current", handlerToken, "", ""))
	if current.ID != created.ID {
		t.Fatalf("current id = %d, want %d", current.ID, created.ID)
	}

	byID := decodePlan(t, callHandler(t, fixture.handler, "GET", fmt.Sprintf("/api/plans/%d", created.ID), handlerToken, "", ""))
	if byID.ID != created.ID {
		t.Fatalf("by-id plan = %+v", byID)
	}

	requireError(t, callHandler(t, fixture.handler, "GET", "/api/plans/999999", handlerToken, "", ""), 404, "not_found")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/plans/not-a-number", handlerToken, "", ""), 404, "not_found")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/plans/current", "", "", ""), 401, "unauthorized")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/unknown", handlerToken, "", ""), 404, "not_found")
}

func TestPlanCreateRejections(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.createExercise(t, "goblet-squat", 1)
	fixture.createExercise(t, "double-dumbbell", 2)

	base := planBody("goblet-squat")
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "unknown exercise", body: planBody("missing"), status: 422, code: "unknown_exercise"},
		{name: "unavailable equipment", body: planBody("double-dumbbell"), status: 422, code: "unavailable_equipment"},
		{name: "equipment limit", body: strings.Replace(base, `"target_kg":20`, `"target_kg":21`, 1), status: 422, code: "equipment_limit"},
		{name: "invalid schedule", body: strings.Replace(base, `"scheduled_date":"2026-12-02"`, `"scheduled_date":"2026-12-02","weekday":2`, 1), status: 422, code: "invalid_schedule"},
		{name: "invalid target", body: strings.Replace(base, `"reps_min":8,"reps_max":12`, `"reps_min":12,"reps_max":8`, 1), status: 422, code: "invalid_target"},
		{name: "invalid interval steps", body: `{"reason":"x","effective_from":"2026-12-01","timezone":"Europe/Madrid","weekly_frequency":3,"sessions":[{"id":"bad","position":0,"weekday":2,"discipline":"intervals","target":"pace"}]}`, status: 422, code: "invalid_interval_steps"},
		{name: "invalid input", body: strings.Replace(base, `"reason":"base strength block"`, `"reason":" "`, 1), status: 400, code: "invalid_input"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "reject-"+test.name, test.body)
			requireError(t, recorder, test.status, test.code)
		})
	}

	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "", base), 400, "idempotency_key_required")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, strings.Repeat("k", 129), base), 400, "idempotency_key_required")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "unknown-field", strings.Replace(base, `"reason"`, `"unknown":true,"reason"`, 1)), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "trailing", base+"{}"), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/plans", handlerToken, "large", `{"reason":"`+strings.Repeat("a", maxBodyBytes)+`"}`), 413, "body_too_large")
}

func TestRuleErrorStatusMapping(t *testing.T) {
	tests := []struct {
		code   string
		status int
		output string
	}{
		{code: "invalid_input", status: 400, output: "invalid_input"},
		{code: "invalid_date", status: 400, output: "invalid_date"},
		{code: "idempotency_key_required", status: 400, output: "idempotency_key_required"},
		{code: "not_found", status: 404, output: "not_found"},
		{code: "idempotency_conflict", status: 409, output: "idempotency_conflict"},
		{code: "exercise_exists", status: 409, output: "exercise_exists"},
		{code: "unknown_exercise", status: 422, output: "unknown_exercise"},
		{code: "unavailable_equipment", status: 422, output: "unavailable_equipment"},
		{code: "equipment_limit", status: 422, output: "equipment_limit"},
		{code: "invalid_schedule", status: 422, output: "invalid_schedule"},
		{code: "invalid_target", status: 422, output: "invalid_target"},
		{code: "invalid_interval_steps", status: 422, output: "invalid_interval_steps"},
		{code: "unknown_equipment", status: 422, output: "unknown_equipment"},
		{code: "revision_conflict", status: 500, output: "internal_error"},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeServiceError(recorder, kernel.NewRuleError(test.code, "message"))
			requireError(t, recorder, test.status, test.output)
		})
	}

	recorder := httptest.NewRecorder()
	writeServiceError(recorder, errors.New("database is down"))
	requireError(t, recorder, 500, "internal_error")
}

func TestCatalogueRoutes(t *testing.T) {
	fixture := newTestFixture(t)

	inventoryRecorder := callHandler(t, fixture.handler, "GET", "/api/inventory", handlerToken, "", "")
	if inventoryRecorder.Code != 200 {
		t.Fatalf("inventory status = %d; body = %s", inventoryRecorder.Code, inventoryRecorder.Body.String())
	}
	var inventory inventoryResponse
	if err := json.Unmarshal(inventoryRecorder.Body.Bytes(), &inventory); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	if len(inventory.Items) != 1 {
		t.Fatalf("inventory items = %+v", inventory.Items)
	}
	item := inventory.Items[0]
	if item.EquipmentID != "adjustable-dumbbell" || item.Name != "Adjustable dumbbell" || item.Quantity != 1 || item.MaxKG == nil || *item.MaxKG != 20 {
		t.Fatalf("inventory item = %+v", item)
	}

	exerciseBody := `{"name":"Goblet squat","movement_pattern":"squat","unilateral":false,"equipment":[{"equipment_id":"adjustable-dumbbell","quantity":1}]}`
	createdRecorder := callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-1", exerciseBody)
	if createdRecorder.Code != 201 {
		t.Fatalf("create exercise status = %d; body = %s", createdRecorder.Code, createdRecorder.Body.String())
	}
	var created exerciseEnvelope
	if err := json.Unmarshal(createdRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode exercise: %v", err)
	}
	if created.Exercise.ID != "goblet-squat" || created.Exercise.Name != "Goblet squat" || created.Exercise.MovementPattern != "squat" {
		t.Fatalf("created exercise = %+v", created.Exercise)
	}
	if created.Exercise.CreatedAt != fixture.clock.at.UnixMilli() {
		t.Fatalf("created_at = %d, want %d", created.Exercise.CreatedAt, fixture.clock.at.UnixMilli())
	}
	if len(created.Exercise.Equipment) != 1 || created.Exercise.Equipment[0].EquipmentID != "adjustable-dumbbell" {
		t.Fatalf("exercise equipment = %+v", created.Exercise.Equipment)
	}

	requireError(t, callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-2", exerciseBody), 409, "exercise_exists")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-3", `{"name":"Ghost press","movement_pattern":"press","unilateral":false,"equipment":[{"equipment_id":"ghost","quantity":1}]}`), 422, "unknown_equipment")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "", exerciseBody), 400, "idempotency_key_required")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-4", strings.Replace(exerciseBody, `"name"`, `"unknown":true,"name"`, 1)), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-5", `{"name":"`+strings.Repeat("a", maxBodyBytes)+`"}`), 413, "body_too_large")

	secondRecorder := callHandler(t, fixture.handler, "POST", "/api/exercises", handlerToken, "exercise-6", `{"name":"Front squat","movement_pattern":"squat","unilateral":false,"equipment":[{"equipment_id":"adjustable-dumbbell","quantity":1}]}`)
	if secondRecorder.Code != 201 {
		t.Fatalf("create second exercise status = %d; body = %s", secondRecorder.Code, secondRecorder.Body.String())
	}

	listRecorder := callHandler(t, fixture.handler, "GET", "/api/exercises", handlerToken, "", "")
	if listRecorder.Code != 200 {
		t.Fatalf("list status = %d; body = %s", listRecorder.Code, listRecorder.Body.String())
	}
	var list exerciseListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode exercise list: %v", err)
	}
	if len(list.Exercises) != 2 || list.NextCursor != nil {
		t.Fatalf("exercise list = %+v", list)
	}
	if list.Exercises[0].ID != "front-squat" || list.Exercises[1].ID != "goblet-squat" {
		t.Fatalf("exercise order = %+v", list.Exercises)
	}

	cursorRecorder := callHandler(t, fixture.handler, "GET", "/api/exercises?cursor=front-squat", handlerToken, "", "")
	if cursorRecorder.Code != 200 {
		t.Fatalf("cursor status = %d; body = %s", cursorRecorder.Code, cursorRecorder.Body.String())
	}
	var cursorList exerciseListResponse
	if err := json.Unmarshal(cursorRecorder.Body.Bytes(), &cursorList); err != nil {
		t.Fatalf("decode cursor list: %v", err)
	}
	if len(cursorList.Exercises) != 1 || cursorList.Exercises[0].ID != "goblet-squat" {
		t.Fatalf("cursor list = %+v", cursorList.Exercises)
	}

	requireError(t, callHandler(t, fixture.handler, "GET", "/api/exercises?cursor=missing", handlerToken, "", ""), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/inventory", "", "", ""), 401, "unauthorized")
}

func TestWeekRoutes(t *testing.T) {
	fixture := newTestFixture(t)

	created, err := fixture.plans.CreatePlan(context.Background(), app.CreatePlanCommand{
		IdempotencyKey: "week-plan",
		Plan: domain.Plan{
			Reason:          "autumn block",
			EffectiveFrom:   "2026-10-01",
			Timezone:        "Europe/Madrid",
			WeeklyFrequency: 3,
			Sessions: []domain.Session{
				{ID: "dated", Position: 0, ScheduledDate: stringPointer("2026-10-20"), Discipline: domain.DisciplineStrength, Target: "full body"},
				{ID: "weekly", Position: 1, Weekday: intPointer(7), Discipline: domain.DisciplineRun, Target: "long run"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create week plan: %v", err)
	}

	fixture.createDay(t, "2026-10-19", "day-1", 25200000, 8000, 52)
	fixture.createDay(t, "2026-10-25", "day-2", 24000000, 10000, 50)

	recorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-10-25", handlerToken, "", "")
	if recorder.Code != 200 {
		t.Fatalf("week status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
	var week weekResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode week: %v", err)
	}
	if week.WeekStart != "2026-10-19" || week.WeekEnd != "2026-10-25" {
		t.Fatalf("week bounds = %s..%s", week.WeekStart, week.WeekEnd)
	}
	if len(week.Planned) != 2 {
		t.Fatalf("planned = %+v", week.Planned)
	}
	if week.Planned[0].Date != "2026-10-20" || week.Planned[0].TemplateID != "dated" || week.Planned[0].PlanVersionID != created.ID {
		t.Fatalf("dated planned entry = %+v", week.Planned[0])
	}
	if week.Planned[1].Date != "2026-10-25" || week.Planned[1].TemplateID != "weekly" || week.Planned[1].Discipline != domain.DisciplineRun {
		t.Fatalf("weekly planned entry = %+v", week.Planned[1])
	}
	if week.Performed == nil || len(week.Performed) != 0 {
		t.Fatalf("performed = %+v, want an empty array", week.Performed)
	}
	if len(week.Days) != 2 {
		t.Fatalf("days = %+v", week.Days)
	}
	if week.Days[0].Date != "2026-10-19" || week.Days[0].Revision != 1 || week.Days[0].SleepMS == nil || *week.Days[0].SleepMS != 25200000 {
		t.Fatalf("first day = %+v", week.Days[0])
	}
	if week.Days[1].Date != "2026-10-25" || week.Days[1].Steps == nil || *week.Days[1].Steps != 10000 || week.Days[1].HRVRMSSDMS != nil {
		t.Fatalf("second day = %+v", week.Days[1])
	}

	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw week: %v", err)
	}
	planned, ok := raw["planned"].([]any)
	if !ok || len(planned) != 2 {
		t.Fatalf("raw planned = %+v", raw["planned"])
	}
	for _, entry := range planned {
		fields, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("raw planned entry = %+v", entry)
		}
		if len(fields) != 5 {
			t.Fatalf("planned entry carries unexpected fields: %+v", fields)
		}
	}

	noPlanRecorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-09-01", handlerToken, "", "")
	if noPlanRecorder.Code != 200 {
		t.Fatalf("no-plan week status = %d; body = %s", noPlanRecorder.Code, noPlanRecorder.Body.String())
	}
	var noPlan weekResponse
	if err := json.Unmarshal(noPlanRecorder.Body.Bytes(), &noPlan); err != nil {
		t.Fatalf("decode no-plan week: %v", err)
	}
	if len(noPlan.Planned) != 0 || len(noPlan.Days) != 0 {
		t.Fatalf("no-plan week = %+v", noPlan)
	}

	requireError(t, callHandler(t, fixture.handler, "GET", "/api/week/2026-02-30", handlerToken, "", ""), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/week/not-a-date", handlerToken, "", ""), 400, "invalid_input")
	requireError(t, callHandler(t, fixture.handler, "GET", "/api/week/2026-10-25", "", "", ""), 401, "unauthorized")
}

func TestWeekPerformedSessions(t *testing.T) {
	fixture := newTestFixture(t)

	if _, err := fixture.plans.CreatePlan(context.Background(), app.CreatePlanCommand{
		IdempotencyKey: "performed-plan",
		Plan: domain.Plan{
			Reason:          "performed block",
			EffectiveFrom:   "2026-10-01",
			Timezone:        "UTC",
			WeeklyFrequency: 2,
			Sessions: []domain.Session{
				{ID: "s1", Position: 0, ScheduledDate: stringPointer("2026-10-21"), Discipline: domain.DisciplineStrength, Target: "full body"},
			},
		},
	}); err != nil {
		t.Fatalf("create performed plan: %v", err)
	}

	templateID := "s1"
	reported := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC).UnixMilli()
	completedStarted := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC).UnixMilli()
	completed, err := fixture.training.CreateSession(context.Background(), trainingapp.CreateSessionCommand{
		IdempotencyKey: "performed-completed",
		ID:             "session-completed",
		TemplateID:     &templateID,
		Date:           "2026-10-21",
		Discipline:     trainingdomain.DisciplineStrength,
		StartedAt:      &completedStarted,
	})
	if err != nil {
		t.Fatalf("create completed session: %v", err)
	}
	if _, err := fixture.training.CheckIn(context.Background(), trainingapp.CheckInCommand{
		SessionID:           completed.ID,
		ExpectedRevision:    1,
		ReportedCompletedAt: &reported,
	}); err != nil {
		t.Fatalf("check in completed session: %v", err)
	}

	incompleteStarted := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC).UnixMilli()
	if _, err := fixture.training.CreateSession(context.Background(), trainingapp.CreateSessionCommand{
		IdempotencyKey: "performed-incomplete",
		ID:             "session-incomplete",
		TemplateID:     &templateID,
		Date:           "2026-10-21",
		Discipline:     trainingdomain.DisciplineStrength,
		StartedAt:      &incompleteStarted,
	}); err != nil {
		t.Fatalf("create incomplete session: %v", err)
	}

	recorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-10-21", handlerToken, "", "")
	if recorder.Code != 200 {
		t.Fatalf("week status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
	var week weekResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode week: %v", err)
	}
	if len(week.Performed) != 2 {
		t.Fatalf("performed = %+v, want 2 entries", week.Performed)
	}

	incomplete := week.Performed[0]
	if incomplete.ID != "session-incomplete" || incomplete.Date != "2026-10-21" || incomplete.StartedAt == nil || *incomplete.StartedAt != "2026-10-21T08:00:00Z" {
		t.Fatalf("first performed entry = %+v", incomplete)
	}
	if incomplete.TemplateID == nil || *incomplete.TemplateID != templateID || incomplete.Discipline != trainingdomain.DisciplineStrength {
		t.Fatalf("first performed metadata = %+v", incomplete)
	}
	if incomplete.Revision != 1 || incomplete.Complete || incomplete.Evidence != nil {
		t.Fatalf("first performed completion = %+v", incomplete)
	}

	complete := week.Performed[1]
	if complete.ID != "session-completed" || complete.Date != "2026-10-21" || complete.StartedAt == nil || *complete.StartedAt != "2026-10-21T09:00:00Z" {
		t.Fatalf("second performed entry = %+v", complete)
	}
	if complete.Revision != 2 || !complete.Complete {
		t.Fatalf("second performed completion = %+v", complete)
	}
	if complete.Evidence == nil || complete.Evidence.Source != trainingdomain.EvidenceReported || complete.Evidence.ReportedCompletedAt == nil || *complete.Evidence.ReportedCompletedAt != "2026-10-21T10:00:00Z" {
		t.Fatalf("second performed evidence = %+v", complete.Evidence)
	}
	if complete.Evidence.WorkoutID != nil {
		t.Fatalf("reported evidence carries workout id: %+v", complete.Evidence)
	}

	if len(week.Planned) != 1 || week.Planned[0].TemplateID != templateID || week.Planned[0].Date != "2026-10-21" {
		t.Fatalf("planned = %+v", week.Planned)
	}
	if !strings.Contains(recorder.Body.String(), `"evidence":null`) {
		t.Fatalf("incomplete session does not carry null evidence: %s", recorder.Body.String())
	}
}

func TestWeekPerformedExcludesSessionsOutsideTheWeek(t *testing.T) {
	fixture := newTestFixture(t)

	for _, test := range []struct {
		id   string
		date string
	}{
		{id: "sunday-before", date: "2026-10-18"},
		{id: "monday-after", date: "2026-10-26"},
	} {
		if _, err := fixture.training.CreateSession(context.Background(), trainingapp.CreateSessionCommand{
			IdempotencyKey: "outside-" + test.id,
			ID:             test.id,
			Date:           test.date,
			Discipline:     trainingdomain.DisciplineRun,
		}); err != nil {
			t.Fatalf("create session %s: %v", test.id, err)
		}
	}

	recorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-10-25", handlerToken, "", "")
	if recorder.Code != 200 {
		t.Fatalf("week status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
	var week weekResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode week: %v", err)
	}
	if week.Performed == nil || len(week.Performed) != 0 {
		t.Fatalf("performed = %+v, want an empty array", week.Performed)
	}
	if !strings.Contains(recorder.Body.String(), `"performed":[]`) {
		t.Fatalf("performed is not an empty array: %s", recorder.Body.String())
	}
}

func TestWeekYearBoundary(t *testing.T) {
	fixture := newTestFixture(t)

	if _, err := fixture.plans.CreatePlan(context.Background(), app.CreatePlanCommand{
		IdempotencyKey: "year-boundary",
		Plan: domain.Plan{
			Reason:          "year boundary block",
			EffectiveFrom:   "2026-12-01",
			Timezone:        "Europe/Madrid",
			WeeklyFrequency: 2,
			Sessions: []domain.Session{
				{ID: "last-of-december", Position: 0, ScheduledDate: stringPointer("2026-12-30"), Discipline: domain.DisciplineStrength, Target: "full body"},
				{ID: "first-saturday", Position: 1, Weekday: intPointer(6), Discipline: domain.DisciplineRun, Target: "repetitions"},
			},
		},
	}); err != nil {
		t.Fatalf("create year boundary plan: %v", err)
	}

	recorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-12-31", handlerToken, "", "")
	if recorder.Code != 200 {
		t.Fatalf("week status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
	var week weekResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode week: %v", err)
	}
	if week.WeekStart != "2026-12-28" || week.WeekEnd != "2027-01-03" {
		t.Fatalf("week bounds = %s..%s", week.WeekStart, week.WeekEnd)
	}
	if len(week.Planned) != 2 {
		t.Fatalf("planned = %+v", week.Planned)
	}
	if week.Planned[0].Date != "2026-12-30" || week.Planned[1].Date != "2027-01-02" {
		t.Fatalf("year boundary planned = %+v", week.Planned)
	}
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func TestWeekSelectsTheVersionEffectiveOnEachDate(t *testing.T) {
	fixture := newTestFixture(t)

	first, err := fixture.plans.CreatePlan(context.Background(), app.CreatePlanCommand{
		IdempotencyKey: "version-one",
		Plan: domain.Plan{
			Reason:          "first block",
			EffectiveFrom:   "2026-10-01",
			Timezone:        "Europe/Madrid",
			WeeklyFrequency: 2,
			Sessions: []domain.Session{
				{ID: "v1-dated", Position: 0, ScheduledDate: stringPointer("2026-10-20"), Discipline: domain.DisciplineStrength, Target: "full body"},
				{ID: "v1-wednesday", Position: 1, Weekday: intPointer(3), Discipline: domain.DisciplineStrength, Target: "push"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create first version: %v", err)
	}

	second, err := fixture.plans.CreatePlan(context.Background(), app.CreatePlanCommand{
		IdempotencyKey: "version-two",
		Plan: domain.Plan{
			Reason:          "second block",
			EffectiveFrom:   "2026-10-22",
			Timezone:        "Europe/Madrid",
			WeeklyFrequency: 2,
			Sessions: []domain.Session{
				{ID: "v2-dated", Position: 0, ScheduledDate: stringPointer("2026-10-24"), Discipline: domain.DisciplineStrength, Target: "pull"},
				{ID: "v2-friday", Position: 1, Weekday: intPointer(5), Discipline: domain.DisciplineStrength, Target: "legs"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create second version: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("both versions share id %d", first.ID)
	}

	recorder := callHandler(t, fixture.handler, "GET", "/api/week/2026-10-21", handlerToken, "", "")
	if recorder.Code != 200 {
		t.Fatalf("week status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
	var week weekResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode week: %v", err)
	}

	expected := []struct {
		date      string
		template  string
		versionID int64
	}{
		{"2026-10-20", "v1-dated", first.ID},
		{"2026-10-21", "v1-wednesday", first.ID},
		{"2026-10-23", "v2-friday", second.ID},
		{"2026-10-24", "v2-dated", second.ID},
	}
	if len(week.Planned) != len(expected) {
		t.Fatalf("planned = %+v, want %d entries", week.Planned, len(expected))
	}
	for index, want := range expected {
		got := week.Planned[index]
		if got.Date != want.date || got.TemplateID != want.template || got.PlanVersionID != want.versionID {
			t.Fatalf("planned[%d] = %+v, want %s/%s/%d", index, got, want.date, want.template, want.versionID)
		}
	}
}
