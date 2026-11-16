package acceptance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	planssqlite "github.com/hectorruiz23/temper/api/internal/plans/adapters/sqlite"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	plansdomain "github.com/hectorruiz23/temper/api/internal/plans/domain"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/migrations"
)

type sessionsClock struct {
	at time.Time
}

func (c sessionsClock) Now() time.Time {
	return c.at
}

func sessionIntPtr(value int) *int {
	return &value
}

func sessionFloatPtr(value float64) *float64 {
	return &value
}

func sessionStringPtr(value string) *string {
	return &value
}

func seedTrainingPlan(t *testing.T, dbPath string) string {
	t.Helper()

	database, err := platformdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open training fixture database: %v", err)
	}
	defer database.Close()

	if err := platformdb.Migrate(context.Background(), database, migrations.FS); err != nil {
		t.Fatalf("migrate training fixture database: %v", err)
	}

	clock := sessionsClock{at: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	store := planssqlite.New(database)
	catalogue := plansapp.NewCatalogueService(store, clock)
	if _, err := catalogue.CreateExercise(context.Background(), plansapp.CreateExerciseCommand{
		ID:              "goblet-squat",
		Name:            "Goblet squat",
		MovementPattern: "squat",
		Equipment:       []plansdomain.ExerciseEquipment{{EquipmentID: "adjustable-dumbbell", Quantity: 1}},
	}); err != nil {
		t.Fatalf("create fixture exercise: %v", err)
	}

	templateID := "template-strength"
	plan := plansdomain.Plan{
		Reason:          "training acceptance plan",
		EffectiveFrom:   "2026-12-01",
		Timezone:        "Europe/Madrid",
		WeeklyFrequency: 3,
		GoalID:          "race-5k-2026-12-20",
		Sessions: []plansdomain.Session{
			{
				ID:            templateID,
				Position:      0,
				ScheduledDate: sessionStringPtr("2026-12-02"),
				Discipline:    plansdomain.DisciplineStrength,
				Target:        "full body",
				Exercises: []plansdomain.SessionExercise{
					{
						Position:   0,
						ExerciseID: "goblet-squat",
						TargetSets: sessionIntPtr(3),
						RepsMin:    sessionIntPtr(8),
						RepsMax:    sessionIntPtr(12),
						TargetKG:   sessionFloatPtr(20),
					},
				},
			},
		},
	}
	if _, err := plansapp.NewPlanService(store, store, clock).CreatePlan(context.Background(), plansapp.CreatePlanCommand{
		IdempotencyKey: "training-acceptance-plan",
		Plan:           plan,
	}); err != nil {
		t.Fatalf("create fixture plan: %v", err)
	}

	return templateID
}

func postTrainingJSON(t *testing.T, url, token, key, body string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if token != "" {
		request.Header.Set("X-Temper-Token", token)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response from %s: %v", url, err)
	}
	return response.StatusCode, strings.TrimSpace(string(content))
}

type trainingSessionPayload struct {
	ID                  string  `json:"id"`
	TemplateID          *string `json:"template_id"`
	Date                string  `json:"date"`
	Discipline          string  `json:"discipline"`
	StartedAt           *string `json:"started_at"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
	CreatedAt           string  `json:"created_at"`
	Revision            int64   `json:"revision"`
	Complete            bool    `json:"complete"`
}

type trainingSessionEnvelope struct {
	Session trainingSessionPayload `json:"session"`
}

type trainingEvidencePayload struct {
	Source              string  `json:"source"`
	WorkoutID           *string `json:"workout_id"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
}

type trainingCheckInEnvelope struct {
	Session           trainingSessionPayload   `json:"session"`
	EffectiveEvidence *trainingEvidencePayload `json:"effective_evidence"`
}

type trainingSetPayload struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	ExerciseID     string  `json:"exercise_id"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	CreatedAt      string  `json:"created_at"`
	Revision       int64   `json:"revision"`
}

type trainingBatchEnvelope struct {
	Sets []trainingSetPayload `json:"sets"`
	Echo struct {
		SessionID string                   `json:"session_id"`
		Sets      []trainingEchoSetPayload `json:"sets"`
	} `json:"echo"`
}

type trainingEchoSetPayload struct {
	ExerciseID string  `json:"exercise_id"`
	SetIndex   int     `json:"set_index"`
	KG         float64 `json:"kg"`
	Reps       int     `json:"reps"`
	Note       *string `json:"note"`
}

func decodeTrainingSession(t *testing.T, body string) trainingSessionPayload {
	t.Helper()

	var envelope trainingSessionEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode session %q: %v", body, err)
	}
	return envelope.Session
}

func decodeTrainingCheckIn(t *testing.T, body string) trainingCheckInEnvelope {
	t.Helper()

	var envelope trainingCheckInEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode check-in %q: %v", body, err)
	}
	return envelope
}

func decodeTrainingBatch(t *testing.T, body string) trainingBatchEnvelope {
	t.Helper()

	var envelope trainingBatchEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode batch %q: %v", body, err)
	}
	return envelope
}

func TestSessionsSetsAndCompletionEvidence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "temper.db")
	templateID := seedTrainingPlan(t, dbPath)
	token := randomToken(t)

	process := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	createBody := fmt.Sprintf(`{"id":"session-a","template_id":%q,"date":"2026-12-02","discipline":"strength","started_at":"2026-12-02T08:00:00Z"}`, templateID)
	status, body := postTrainingJSON(t, baseURL+"/api/sessions", token, "create-a", createBody)
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %s, want 201", status, body)
	}
	created := decodeTrainingSession(t, body)
	if created.ID != "session-a" || created.Revision != 1 || created.Complete {
		t.Fatalf("created session = %+v", created)
	}
	if created.TemplateID == nil || *created.TemplateID != templateID {
		t.Fatalf("template id = %v", created.TemplateID)
	}
	if created.WatchWorkoutID != nil || created.ReportedCompletedAt != nil {
		t.Fatalf("create wrote evidence: %+v", created)
	}
	if created.StartedAt == nil || *created.StartedAt != "2026-12-02T08:00:00Z" {
		t.Fatalf("started_at = %v", created.StartedAt)
	}
	if created.CreatedAt == "" {
		t.Fatal("created_at is empty")
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sessions", token, "create-invalid", `{"id":"session-invalid","date":"2026-02-30","discipline":"strength"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid session = %d %s, want 400", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	plannedBody := fmt.Sprintf(`{"id":"session-planned","template_id":%q,"date":"2026-12-04","discipline":"strength"}`, templateID)
	status, body = postTrainingJSON(t, baseURL+"/api/sessions", token, "create-planned", plannedBody)
	if status != http.StatusCreated {
		t.Fatalf("create planned session = %d %s, want 201", status, body)
	}
	planned := decodeTrainingSession(t, body)
	if planned.Complete || planned.WatchWorkoutID != nil || planned.ReportedCompletedAt != nil {
		t.Fatalf("planned session is complete on creation: %+v", planned)
	}

	batchBody := `{"session_id":"session-a","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20.005,"reps":10,"note":"  felt good  "},{"exercise_id":"goblet-squat","set_index":2,"kg":20,"reps":8,"note":"   "}]}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-a", batchBody)
	if status != http.StatusCreated {
		t.Fatalf("log sets = %d %s, want 201", status, body)
	}
	batch := decodeTrainingBatch(t, body)
	if len(batch.Sets) != 2 || batch.Echo.SessionID != "session-a" || len(batch.Echo.Sets) != 2 {
		t.Fatalf("batch = %+v", batch)
	}
	if batch.Echo.Sets[0].ExerciseID != "goblet-squat" || batch.Echo.Sets[0].KG != 20.01 || batch.Echo.Sets[0].Note == nil || *batch.Echo.Sets[0].Note != "felt good" {
		t.Fatalf("echo = %+v", batch.Echo.Sets)
	}
	if batch.Sets[0].KG != 20.01 || batch.Sets[0].Reps != 10 || batch.Sets[0].Note == nil || *batch.Sets[0].Note != "felt good" {
		t.Fatalf("first set = %+v", batch.Sets[0])
	}
	if batch.Sets[0].EvidenceSource != "reported" || batch.Sets[0].Revision != 1 || batch.Sets[0].CreatedAt == "" {
		t.Fatalf("first set metadata = %+v", batch.Sets[0])
	}
	if batch.Sets[1].Note != nil {
		t.Fatalf("blank note = %v, want nil", *batch.Sets[1].Note)
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-a", batchBody)
	if status != http.StatusCreated {
		t.Fatalf("replay sets = %d %s, want 201", status, body)
	}
	replayed := decodeTrainingBatch(t, body)
	if !reflect.DeepEqual(replayed, batch) {
		t.Fatalf("replay = %+v, want %+v", replayed, batch)
	}

	changedBody := strings.Replace(batchBody, `"reps":10`, `"reps":9`, 1)
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-a", changedBody)
	if status != http.StatusConflict {
		t.Fatalf("idempotency conflict = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "idempotency_conflict")

	invalidBatchBody := `{"session_id":"session-a","sets":[{"exercise_id":"goblet-squat","set_index":3,"kg":20,"reps":0}]}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-invalid", invalidBatchBody)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid set = %d %s, want 422", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	occupiedBody := `{"session_id":"session-a","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20,"reps":10}]}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-occupied", occupiedBody)
	if status != http.StatusConflict {
		t.Fatalf("occupied position = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "set_position_occupied")

	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-a/check-in", token, "", `{"expected_revision":1,"reported_completed_at":"2026-12-02T09:00:00Z","watch_workout_id":"both"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("check-in with both sources = %d %s, want 422", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-a/check-in", token, "", `{"expected_revision":1}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("check-in without evidence = %d %s, want 422", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-a/check-in", token, "", `{"expected_revision":1,"reported_completed_at":"2026-12-02T09:00:00Z"}`)
	if status != http.StatusOK {
		t.Fatalf("check in reported = %d %s, want 200", status, body)
	}
	checkedIn := decodeTrainingCheckIn(t, body)
	if !checkedIn.Session.Complete || checkedIn.Session.Revision != 2 {
		t.Fatalf("checked in session = %+v", checkedIn.Session)
	}
	if checkedIn.Session.ReportedCompletedAt == nil || *checkedIn.Session.ReportedCompletedAt != "2026-12-02T09:00:00Z" {
		t.Fatalf("reported_completed_at = %v", checkedIn.Session.ReportedCompletedAt)
	}
	if checkedIn.EffectiveEvidence == nil || checkedIn.EffectiveEvidence.Source != "reported" || checkedIn.EffectiveEvidence.ReportedCompletedAt == nil {
		t.Fatalf("effective evidence = %+v", checkedIn.EffectiveEvidence)
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-a/check-in", token, "", `{"expected_revision":1,"reported_completed_at":"2026-12-02T09:00:00Z"}`)
	if status != http.StatusConflict {
		t.Fatalf("stale check-in = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "revision_conflict")

	workoutStart := time.Date(2026, 12, 3, 8, 0, 0, 0, time.UTC)
	workoutEnd := workoutStart.Add(45 * time.Minute)
	dayBody := fmt.Sprintf(`{"date":"2026-12-03","timezone":"Europe/Madrid","expected_revision":0,"source":"health_connect","workouts":[{"origin":"health_connect","external_id":"watch-session","device_kind":"watch","started_at":%d,"ended_at":%d,"discipline":"strength","duration_ms":2700000,"watch_verified":true}]}`, workoutStart.UnixMilli(), workoutEnd.UnixMilli())
	status, body = postDay(t, baseURL, token, "day-watch", dayBody)
	if status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, body)
	}
	day := decodeDay(t, body)
	if len(day.Day.Workouts) != 1 || day.Day.Workouts[0].ID == "" {
		t.Fatalf("day workouts = %+v", day.Day.Workouts)
	}
	workoutID := day.Day.Workouts[0].ID

	watchCreate := fmt.Sprintf(`{"id":"session-watch","template_id":%q,"date":"2026-12-03","discipline":"strength"}`, templateID)
	status, body = postTrainingJSON(t, baseURL+"/api/sessions", token, "create-watch", watchCreate)
	if status != http.StatusCreated {
		t.Fatalf("create watched session = %d %s, want 201", status, body)
	}
	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-watch/check-in", token, "", fmt.Sprintf(`{"expected_revision":1,"watch_workout_id":%q}`, workoutID))
	if status != http.StatusOK {
		t.Fatalf("check in watch = %d %s, want 200", status, body)
	}
	watched := decodeTrainingCheckIn(t, body)
	if !watched.Session.Complete || watched.Session.WatchWorkoutID == nil || *watched.Session.WatchWorkoutID != workoutID {
		t.Fatalf("watched session = %+v", watched.Session)
	}
	if watched.Session.ReportedCompletedAt != nil {
		t.Fatalf("watch check-in kept reported evidence: %+v", watched.Session)
	}
	if watched.EffectiveEvidence == nil || watched.EffectiveEvidence.Source != "watch" || watched.EffectiveEvidence.WorkoutID == nil {
		t.Fatalf("watch evidence = %+v", watched.EffectiveEvidence)
	}

	thirdCreate := fmt.Sprintf(`{"id":"session-third","template_id":%q,"date":"2026-12-03","discipline":"strength"}`, templateID)
	status, body = postTrainingJSON(t, baseURL+"/api/sessions", token, "create-third", thirdCreate)
	if status != http.StatusCreated {
		t.Fatalf("create third session = %d %s, want 201", status, body)
	}
	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-third/check-in", token, "", fmt.Sprintf(`{"expected_revision":1,"watch_workout_id":%q}`, workoutID))
	if status != http.StatusConflict {
		t.Fatalf("workout linked twice = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "workout_already_linked")

	status, body = postTrainingJSON(t, baseURL+"/api/sessions/session-third/check-in", token, "", `{"expected_revision":1,"watch_workout_id":"missing-workout"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("missing workout = %d %s, want 422", status, body)
	}
	requireErrorShape(t, body, "unusable_watch_evidence")

	process.stop(t)

	database := openDatabase(t, dbPath)
	var reported sql.NullInt64
	var watch sql.NullString
	if err := database.QueryRow(`SELECT reported_completed_at, watch_workout_id FROM training_session WHERE id = 'session-planned'`).Scan(&reported, &watch); err != nil {
		t.Fatalf("read planned session: %v", err)
	}
	if reported.Valid || watch.Valid {
		t.Fatalf("planned session gained evidence reported=%v watch=%v", reported, watch)
	}
	if err := database.QueryRow(`SELECT reported_completed_at, watch_workout_id FROM training_session WHERE id = 'session-a'`).Scan(&reported, &watch); err != nil {
		t.Fatalf("read reported session: %v", err)
	}
	if !reported.Valid || watch.Valid {
		t.Fatalf("reported session evidence reported=%v watch=%v", reported, watch)
	}
	if err := database.QueryRow(`SELECT reported_completed_at, watch_workout_id FROM training_session WHERE id = 'session-watch'`).Scan(&reported, &watch); err != nil {
		t.Fatalf("read watched session: %v", err)
	}
	if reported.Valid || !watch.Valid || watch.String != workoutID {
		t.Fatalf("watched session evidence reported=%v watch=%v", reported, watch)
	}

	var sets int
	if err := database.QueryRow(`SELECT COUNT(*) FROM logged_set WHERE session_id = 'session-a'`).Scan(&sets); err != nil {
		t.Fatalf("count sets: %v", err)
	}
	if sets != 2 {
		t.Fatalf("logged sets = %d, want 2", sets)
	}
}
