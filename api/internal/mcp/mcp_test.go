package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	healthsqlite "github.com/hectorruiz23/temper/api/internal/health/adapters/sqlite"
	healthapp "github.com/hectorruiz23/temper/api/internal/health/app"
	planssqlite "github.com/hectorruiz23/temper/api/internal/plans/adapters/sqlite"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	platformdb "github.com/hectorruiz23/temper/api/internal/platform/db"
	trainingsqlite "github.com/hectorruiz23/temper/api/internal/training/adapters/sqlite"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
	"github.com/hectorruiz23/temper/api/migrations"
)

type fixedClock struct {
	at time.Time
}

func (c fixedClock) Now() time.Time {
	return c.at
}

type fixture struct {
	handler *Handler
	db      *sql.DB
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	database, err := platformdb.Open(filepath.Join(t.TempDir(), "mcp.db"))
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

	handler := New(
		plansapp.NewPlanService(planStore, planStore, clock),
		plansapp.NewCatalogueService(planStore, clock),
		healthapp.New(healthStore, clock),
		trainingapp.New(trainingStore, clock),
	)

	return &fixture{handler: handler, db: database}
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

func rpcCall(t *testing.T, handler *Handler, method, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) rpcEnvelope {
	t.Helper()

	var envelope rpcEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", recorder.Body.String(), err)
	}
	return envelope
}

func callTool(t *testing.T, handler *Handler, id int, name string, args any) toolCallResult {
	t.Helper()

	arguments, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, arguments)
	recorder := rpcCall(t, handler, http.MethodPost, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", name, recorder.Code, recorder.Body.String())
	}

	envelope := decodeEnvelope(t, recorder)
	if envelope.Error != nil {
		t.Fatalf("%s returned JSON-RPC error %d: %s", name, envelope.Error.Code, envelope.Error.Message)
	}

	var result toolCallResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode %s result %q: %v", name, envelope.Result, err)
	}
	if result.IsError {
		t.Fatalf("%s returned a tool error: %s", name, result.Content[0].Text)
	}
	return result
}

func structured(t *testing.T, result toolCallResult, target any) {
	t.Helper()

	if err := json.Unmarshal(result.StructuredContent, target); err != nil {
		t.Fatalf("decode structured content %q: %v", result.StructuredContent, err)
	}
}

func TestFraming(t *testing.T) {
	f := newFixture(t)

	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "parse error", body: `{`, code: codeParseError},
		{name: "invalid JSON", body: `not json`, code: codeParseError},
		{name: "batch", body: `[]`, code: codeInvalidRequest},
		{name: "scalar", body: `null`, code: codeInvalidRequest},
		{name: "bad method type", body: `{"jsonrpc":"2.0","id":1,"method":1}`, code: codeInvalidRequest},
		{name: "bad id", body: `{"jsonrpc":"2.0","id":{},"method":"ping"}`, code: codeInvalidRequest},
		{name: "missing jsonrpc", body: `{"id":1,"method":"ping"}`, code: codeInvalidRequest},
		{name: "unknown method", body: `{"jsonrpc":"2.0","id":1,"method":"nope"}`, code: codeMethodNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := rpcCall(t, f.handler, http.MethodPost, test.body)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
			envelope := decodeEnvelope(t, recorder)
			if envelope.Error == nil || envelope.Error.Code != test.code {
				t.Fatalf("error = %+v, want code %d", envelope.Error, test.code)
			}
		})
	}
}

func TestPingAndNotification(t *testing.T) {
	f := newFixture(t)

	recorder := rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":"p","method":"ping"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ping status = %d, want 200", recorder.Code)
	}
	envelope := decodeEnvelope(t, recorder)
	if string(envelope.ID) != `"p"` || string(envelope.Result) != `{}` {
		t.Fatalf("ping response = %+v, want result {}", envelope)
	}

	recorder = rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("notification status = %d, want 202", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("notification body = %q, want empty", recorder.Body.String())
	}
}

func TestGetIsMethodNotAllowed(t *testing.T) {
	f := newFixture(t)

	recorder := rpcCall(t, f.handler, http.MethodGet, "")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", recorder.Code)
	}
	if recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", recorder.Header().Get("Allow"))
	}
}

func TestOriginAndProtocolVersion(t *testing.T) {
	f := newFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("Origin", "https://elsewhere.example")
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("Origin", "http://"+request.Host)
	recorder = httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("same-origin status = %d, want 200", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("MCP-Protocol-Version", "1900-01-01")
	recorder = httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unsupported version status = %d, want 200", recorder.Code)
	}
	envelope := decodeEnvelope(t, recorder)
	if envelope.Error == nil || envelope.Error.Code != codeInvalidRequest {
		t.Fatalf("unsupported version error = %+v, want %d", envelope.Error, codeInvalidRequest)
	}
}

func TestInitialize(t *testing.T) {
	f := newFixture(t)

	envelope := decodeEnvelope(t, rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1900-01-01"}}`))
	if envelope.Error != nil {
		t.Fatalf("initialize error = %+v", envelope.Error)
	}

	var result struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if result.ProtocolVersion != protocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", result.ProtocolVersion, protocolVersion)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Fatalf("capabilities = %+v, want tools", result.Capabilities)
	}
	if result.ServerInfo.Name != "temper" || result.ServerInfo.Version == "" {
		t.Fatalf("serverInfo = %+v", result.ServerInfo)
	}
}

func TestToolsList(t *testing.T) {
	f := newFixture(t)

	envelope := decodeEnvelope(t, rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if envelope.Error != nil {
		t.Fatalf("tools/list error = %+v", envelope.Error)
	}

	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(result.Tools) != 14 {
		t.Fatalf("tool count = %d, want 14", len(result.Tools))
	}

	want := []string{
		"get_inventory", "list_exercises", "create_exercise", "create_plan", "get_current_plan",
		"get_plan", "get_week", "get_day", "upsert_day", "create_session", "check_in_session",
		"log_sets", "correct_set", "get_progression",
	}
	names := make([]string, len(result.Tools))
	for index, tool := range result.Tools {
		names[index] = tool.Name
		if tool.Description == "" {
			t.Fatalf("tool %s has no description", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Fatalf("tool %s inputSchema type = %v, want object", tool.Name, tool.InputSchema["type"])
		}
		properties, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok || properties == nil {
			t.Fatalf("tool %s inputSchema properties = %v, want object", tool.Name, tool.InputSchema["properties"])
		}
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestGetDayCarriesWorkoutMetrics(t *testing.T) {
	f := newFixture(t)

	callTool(t, f.handler, 1, "upsert_day", map[string]any{
		"idempotency_key":   "day-metrics",
		"date":              "2026-11-10",
		"timezone":          "Europe/Madrid",
		"expected_revision": 0,
		"source":            "health_connect",
		"workouts": []map[string]any{
			{
				"origin":           "health_connect",
				"external_id":      "watch-run-1",
				"discipline":       "run",
				"started_at":       1762758000000,
				"ended_at":         1762759740000,
				"duration_ms":      1740000,
				"distance_m":       5230,
				"avg_hr_bpm":       142,
				"max_hr_bpm":       171,
				"calories_kcal":    412.5,
				"cadence_spm":      168,
				"elevation_gain_m": 96,
				"splits": []map[string]any{
					{"position": 0, "kind": "km", "distance_m": 1000, "duration_ms": 330000, "avg_hr_bpm": 138},
				},
			},
		},
	})

	read := callTool(t, f.handler, 2, "get_day", map[string]any{"date": "2026-11-10"})
	var result struct {
		Day struct {
			Workouts []struct {
				AvgHRBPM       *int64   `json:"avg_hr_bpm"`
				MaxHRBPM       *int64   `json:"max_hr_bpm"`
				CaloriesKcal   *float64 `json:"calories_kcal"`
				CadenceSPM     *float64 `json:"cadence_spm"`
				ElevationGainM *float64 `json:"elevation_gain_m"`
				PaceSecPerKM   *float64 `json:"pace_sec_per_km"`
				Splits         []struct {
					Position     int      `json:"position"`
					PaceSecPerKM *float64 `json:"pace_sec_per_km"`
				} `json:"splits"`
			} `json:"workouts"`
		} `json:"day"`
	}
	structured(t, read, &result)
	if len(result.Day.Workouts) != 1 {
		t.Fatalf("workouts = %+v", result.Day.Workouts)
	}
	workout := result.Day.Workouts[0]
	if workout.AvgHRBPM == nil || *workout.AvgHRBPM != 142 || workout.MaxHRBPM == nil || *workout.MaxHRBPM != 171 {
		t.Fatalf("heart rate = %v/%v", workout.AvgHRBPM, workout.MaxHRBPM)
	}
	if workout.CaloriesKcal == nil || *workout.CaloriesKcal != 412.5 {
		t.Fatalf("calories = %v", workout.CaloriesKcal)
	}
	if workout.CadenceSPM == nil || *workout.CadenceSPM != 168 || workout.ElevationGainM == nil || *workout.ElevationGainM != 96 {
		t.Fatalf("cadence/elevation = %v/%v", workout.CadenceSPM, workout.ElevationGainM)
	}
	if workout.PaceSecPerKM == nil || *workout.PaceSecPerKM < 332.6 || *workout.PaceSecPerKM > 332.8 {
		t.Fatalf("pace = %v, want 332.7", workout.PaceSecPerKM)
	}
	if len(workout.Splits) != 1 || workout.Splits[0].PaceSecPerKM == nil || *workout.Splits[0].PaceSecPerKM != 330 {
		t.Fatalf("splits = %+v", workout.Splits)
	}
}

func TestAllToolsAgainstServices(t *testing.T) {
	f := newFixture(t)

	inventory := callTool(t, f.handler, 1, "get_inventory", map[string]any{})
	var inventoryResult struct {
		Items []struct {
			EquipmentID string `json:"equipment_id"`
			Quantity    int    `json:"quantity"`
		} `json:"items"`
	}
	structured(t, inventory, &inventoryResult)
	if len(inventoryResult.Items) != 1 || inventoryResult.Items[0].EquipmentID != "adjustable-dumbbell" {
		t.Fatalf("inventory = %+v", inventoryResult)
	}

	exercise := callTool(t, f.handler, 2, "create_exercise", map[string]any{
		"idempotency_key":  "exercise-goblet-squat",
		"name":             "Goblet squat",
		"movement_pattern": "squat",
		"unilateral":       false,
		"equipment": []map[string]any{
			{"equipment_id": "adjustable-dumbbell", "quantity": 1},
		},
	})
	var exerciseResult struct {
		Exercise struct {
			ID string `json:"id"`
		} `json:"exercise"`
	}
	structured(t, exercise, &exerciseResult)
	if exerciseResult.Exercise.ID != "goblet-squat" {
		t.Fatalf("exercise id = %q", exerciseResult.Exercise.ID)
	}

	listed := callTool(t, f.handler, 3, "list_exercises", map[string]any{})
	var listResult struct {
		Exercises []struct {
			ID string `json:"id"`
		} `json:"exercises"`
	}
	structured(t, listed, &listResult)
	if len(listResult.Exercises) != 1 || listResult.Exercises[0].ID != "goblet-squat" {
		t.Fatalf("exercises = %+v", listResult)
	}

	plan := callTool(t, f.handler, 4, "create_plan", map[string]any{
		"idempotency_key":  "plan-v1",
		"reason":           "base block",
		"effective_from":   "2026-12-01",
		"timezone":         "Europe/Madrid",
		"weekly_frequency": 3,
		"goal_id":          "race-5k-2026-12-20",
		"sessions": []map[string]any{
			{
				"id":             "strength-template",
				"position":       0,
				"scheduled_date": "2026-12-02",
				"discipline":     "strength",
				"target":         "full body",
				"exercises": []map[string]any{
					{"position": 0, "exercise_id": "goblet-squat", "target_sets": 3, "reps_min": 8, "reps_max": 12, "target_kg": 20, "tempo": "3-1-1"},
				},
			},
			{
				"id":         "intervals-template",
				"position":   1,
				"weekday":    3,
				"discipline": "intervals",
				"target":     "5K pace",
				"interval_steps": []map[string]any{
					{"position": 0, "label": "warm up", "duration_ms": 600000},
					{"position": 1, "label": "repetition", "distance_m": 400},
				},
			},
		},
	})
	var planResult struct {
		Plan struct {
			ID       int64 `json:"id"`
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		} `json:"plan"`
	}
	structured(t, plan, &planResult)
	if planResult.Plan.ID <= 0 || len(planResult.Plan.Sessions) != 2 {
		t.Fatalf("plan = %+v", planResult)
	}

	current := callTool(t, f.handler, 5, "get_current_plan", map[string]any{})
	var currentResult struct {
		Plan struct {
			ID int64 `json:"id"`
		} `json:"plan"`
	}
	structured(t, current, &currentResult)
	if currentResult.Plan.ID != planResult.Plan.ID {
		t.Fatalf("current plan = %d, want %d", currentResult.Plan.ID, planResult.Plan.ID)
	}

	byID := callTool(t, f.handler, 6, "get_plan", map[string]any{"id": planResult.Plan.ID})
	var byIDResult struct {
		Plan struct {
			ID int64 `json:"id"`
		} `json:"plan"`
	}
	structured(t, byID, &byIDResult)
	if byIDResult.Plan.ID != planResult.Plan.ID {
		t.Fatalf("plan by id = %d, want %d", byIDResult.Plan.ID, planResult.Plan.ID)
	}

	week := callTool(t, f.handler, 7, "get_week", map[string]any{"date": "2026-12-02"})
	var weekResult struct {
		WeekStart string `json:"week_start"`
		WeekEnd   string `json:"week_end"`
		Planned   []struct {
			TemplateID string `json:"template_id"`
		} `json:"planned"`
	}
	structured(t, week, &weekResult)
	if weekResult.WeekStart != "2026-11-30" || weekResult.WeekEnd != "2026-12-06" || len(weekResult.Planned) != 2 {
		t.Fatalf("week = %+v", weekResult)
	}

	day := callTool(t, f.handler, 8, "upsert_day", map[string]any{
		"idempotency_key":   "day-2026-12-02",
		"date":              "2026-12-02",
		"timezone":          "Europe/Madrid",
		"expected_revision": 0,
		"source":            "manual",
		"metrics": map[string]any{
			"sleep_ms":       25200000,
			"steps":          8000,
			"resting_hr_bpm": 52,
		},
	})
	var dayResult struct {
		Day struct {
			Revision int64 `json:"revision"`
		} `json:"day"`
	}
	structured(t, day, &dayResult)
	if dayResult.Day.Revision != 1 {
		t.Fatalf("day revision = %d, want 1", dayResult.Day.Revision)
	}

	readDay := callTool(t, f.handler, 9, "get_day", map[string]any{"date": "2026-12-02"})
	var readDayResult struct {
		Day struct {
			Step int64 `json:"steps"`
		} `json:"day"`
	}
	structured(t, readDay, &readDayResult)
	if readDayResult.Day.Step != 8000 {
		t.Fatalf("day steps = %d, want 8000", readDayResult.Day.Step)
	}

	session := callTool(t, f.handler, 10, "create_session", map[string]any{
		"idempotency_key": "session-a-create",
		"id":              "session-a",
		"template_id":     "strength-template",
		"date":            "2026-12-02",
		"discipline":      "strength",
		"started_at":      "2026-12-02T08:00:00Z",
	})
	var sessionResult struct {
		Session struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
			Complete bool   `json:"complete"`
		} `json:"session"`
	}
	structured(t, session, &sessionResult)
	if sessionResult.Session.ID != "session-a" || sessionResult.Session.Revision != 1 || sessionResult.Session.Complete {
		t.Fatalf("session = %+v", sessionResult.Session)
	}

	logArgs := map[string]any{
		"idempotency_key": "batch-a",
		"session_id":      "session-a",
		"sets": []map[string]any{
			{"exercise_id": "goblet-squat", "set_index": 1, "kg": 20, "reps": 10, "note": "felt good"},
			{"exercise_id": "goblet-squat", "set_index": 2, "kg": 20, "reps": 8},
			{"exercise_id": "goblet-squat", "set_index": 3, "kg": 18, "reps": 8},
		},
	}
	batch := callTool(t, f.handler, 11, "log_sets", logArgs)
	var batchResult struct {
		Sets []struct {
			ID string `json:"id"`
		} `json:"sets"`
	}
	structured(t, batch, &batchResult)
	if len(batchResult.Sets) != 3 {
		t.Fatalf("logged sets = %d, want 3", len(batchResult.Sets))
	}

	replay := callTool(t, f.handler, 12, "log_sets", logArgs)
	if string(replay.StructuredContent) != string(batch.StructuredContent) {
		t.Fatalf("replay changed the result:\nfirst: %s\nsecond: %s", batch.StructuredContent, replay.StructuredContent)
	}
	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM logged_set WHERE session_id = 'session-a'`).Scan(&count); err != nil {
		t.Fatalf("count sets: %v", err)
	}
	if count != 3 {
		t.Fatalf("logged_set rows = %d, want 3", count)
	}

	checkIn := callTool(t, f.handler, 13, "check_in_session", map[string]any{
		"idempotency_key":       "check-in-a",
		"session_id":            "session-a",
		"expected_revision":     1,
		"reported_completed_at": "2026-12-02T09:00:00Z",
	})
	var checkInResult struct {
		Session struct {
			Complete bool  `json:"complete"`
			Revision int64 `json:"revision"`
		} `json:"session"`
		EffectiveEvidence struct {
			Source string `json:"source"`
		} `json:"effective_evidence"`
	}
	structured(t, checkIn, &checkInResult)
	if !checkInResult.Session.Complete || checkInResult.Session.Revision != 2 || checkInResult.EffectiveEvidence.Source != "reported" {
		t.Fatalf("check in = %+v", checkInResult)
	}

	corrected := callTool(t, f.handler, 14, "correct_set", map[string]any{
		"idempotency_key":   "correct-a",
		"set_id":            batchResult.Sets[0].ID,
		"expected_revision": 1,
		"reason":            "wrong weight on the bar",
		"changes":           map[string]any{"kg": 18},
	})
	var correctionResult struct {
		Set struct {
			KG       float64 `json:"kg"`
			Revision int64   `json:"revision"`
		} `json:"set"`
	}
	structured(t, corrected, &correctionResult)
	if correctionResult.Set.KG != 18 || correctionResult.Set.Revision != 2 {
		t.Fatalf("correction = %+v", correctionResult.Set)
	}

	progression := callTool(t, f.handler, 15, "get_progression", map[string]any{"exercise_id": "goblet-squat"})
	var progressionResult struct {
		PerformedHistory []struct {
			KG float64 `json:"kg"`
		} `json:"performed_history"`
		CurrentTargets []struct {
			TemplateID string `json:"template_id"`
		} `json:"current_targets"`
	}
	structured(t, progression, &progressionResult)
	if len(progressionResult.PerformedHistory) != 3 || len(progressionResult.CurrentTargets) != 1 {
		t.Fatalf("progression = %+v", progressionResult)
	}
}

func TestToolErrorAndInvalidParams(t *testing.T) {
	f := newFixture(t)

	recorder := rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_day","arguments":{"date":"2026-01-01"}}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	envelope := decodeEnvelope(t, recorder)
	if envelope.Error != nil {
		t.Fatalf("domain failure became a JSON-RPC error: %+v", envelope.Error)
	}
	var result toolCallResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "not_found") {
		t.Fatalf("missing day result = %+v", result)
	}

	recorder = rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	envelope = decodeEnvelope(t, recorder)
	if envelope.Error == nil || envelope.Error.Code != codeInvalidParams {
		t.Fatalf("unknown tool error = %+v, want %d", envelope.Error, codeInvalidParams)
	}

	recorder = rpcCall(t, f.handler, http.MethodPost, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_plan","arguments":{}}}`)
	envelope = decodeEnvelope(t, recorder)
	if envelope.Error == nil || envelope.Error.Code != codeInvalidParams {
		t.Fatalf("missing argument error = %+v, want %d", envelope.Error, codeInvalidParams)
	}
}
