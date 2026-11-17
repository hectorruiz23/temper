package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mcpRPCError    `json:"error"`
}

type mcpCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

func mcpHTTPResponse(t *testing.T, url, token, body string) (int, string, http.Header) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create MCP request: %v", err)
	}
	if token != "" {
		request.Header.Set("X-Temper-Token", token)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read MCP response: %v", err)
	}

	return response.StatusCode, strings.TrimSpace(string(content)), response.Header
}

func mcpRPC(t *testing.T, baseURL, token, body string) (mcpRPCEnvelope, http.Header) {
	t.Helper()

	status, content, header := mcpHTTPResponse(t, baseURL+"/mcp", token, body)
	if status != http.StatusOK {
		t.Fatalf("MCP status = %d, want 200; body = %s", status, content)
	}

	var envelope mcpRPCEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("decode MCP envelope %q: %v", content, err)
	}
	if envelope.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", envelope.JSONRPC)
	}
	if envelope.Error != nil {
		t.Fatalf("MCP error = %+v; body = %s", envelope.Error, content)
	}

	return envelope, header
}

func mcpToolCall(t *testing.T, baseURL, token string, id int, name string, args any) mcpCallResult {
	t.Helper()

	arguments, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal %s arguments: %v", name, err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, arguments)
	envelope, _ := mcpRPC(t, baseURL, token, body)

	var result mcpCallResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode %s result %q: %v", name, envelope.Result, err)
	}
	if result.IsError {
		t.Fatalf("%s returned a tool error: %s", name, result.Content[0].Text)
	}
	return result
}

func mcpStructured(t *testing.T, result mcpCallResult, target any) {
	t.Helper()

	if err := json.Unmarshal(result.StructuredContent, target); err != nil {
		t.Fatalf("decode structured content %q: %v", result.StructuredContent, err)
	}
}

func TestMCPFullCycleOverTheRealBinary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "temper.db")
	token := randomToken(t)
	process := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	status, _, _ := mcpHTTPResponse(t, baseURL+"/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated MCP status = %d, want 401", status)
	}

	status, body := request(t, baseURL+"/mcp", token)
	if status != http.StatusMethodNotAllowed || body != "" {
		t.Fatalf("GET /mcp = %d %q, want 405 with an empty body", status, body)
	}

	initialize, header := mcpRPC(t, baseURL, token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"acceptance","version":"1"}}}`)
	if header.Get("Mcp-Session-Id") != "" {
		t.Fatalf("initialize emitted Mcp-Session-Id %q", header.Get("Mcp-Session-Id"))
	}
	var initialized struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(initialize.Result, &initialized); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if initialized.ProtocolVersion != "2025-11-25" || initialized.ServerInfo.Name != "temper" || initialized.ServerInfo.Version == "" {
		t.Fatalf("initialize result = %+v", initialized)
	}
	if _, ok := initialized.Capabilities["tools"]; !ok {
		t.Fatalf("initialize capabilities = %+v", initialized.Capabilities)
	}

	status, content, _ := mcpHTTPResponse(t, baseURL+"/mcp", token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted || content != "" {
		t.Fatalf("notification = %d %q, want 202 with an empty body", status, content)
	}

	tools, _ := mcpRPC(t, baseURL, token, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var listed struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(tools.Result, &listed); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(listed.Tools) != 14 {
		t.Fatalf("tool count = %d, want 14", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.InputSchema["type"] != "object" {
			t.Fatalf("tool %s inputSchema = %+v", tool.Name, tool.InputSchema)
		}
	}

	exercise := mcpToolCall(t, baseURL, token, 3, "create_exercise", map[string]any{
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
	mcpStructured(t, exercise, &exerciseResult)
	if exerciseResult.Exercise.ID != "goblet-squat" {
		t.Fatalf("exercise id = %q", exerciseResult.Exercise.ID)
	}

	plan := mcpToolCall(t, baseURL, token, 4, "create_plan", map[string]any{
		"idempotency_key":  "plan-v1",
		"reason":           "base strength block",
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
			ID int64 `json:"id"`
		} `json:"plan"`
	}
	mcpStructured(t, plan, &planResult)
	if planResult.Plan.ID <= 0 {
		t.Fatalf("plan id = %d", planResult.Plan.ID)
	}

	week := mcpToolCall(t, baseURL, token, 5, "get_week", map[string]any{"date": "2026-12-02"})
	var weekResult struct {
		WeekStart string `json:"week_start"`
		WeekEnd   string `json:"week_end"`
		Planned   []struct {
			TemplateID    string `json:"template_id"`
			PlanVersionID int64  `json:"plan_version_id"`
		} `json:"planned"`
	}
	mcpStructured(t, week, &weekResult)
	if weekResult.WeekStart != "2026-11-30" || weekResult.WeekEnd != "2026-12-06" || len(weekResult.Planned) != 2 {
		t.Fatalf("week = %+v", weekResult)
	}
	if weekResult.Planned[0].PlanVersionID != planResult.Plan.ID || weekResult.Planned[1].PlanVersionID != planResult.Plan.ID {
		t.Fatalf("week plan version = %+v, want %d", weekResult.Planned, planResult.Plan.ID)
	}

	session := mcpToolCall(t, baseURL, token, 6, "create_session", map[string]any{
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
	mcpStructured(t, session, &sessionResult)
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
	batch := mcpToolCall(t, baseURL, token, 7, "log_sets", logArgs)
	var batchResult struct {
		Sets []struct {
			ID string `json:"id"`
		} `json:"sets"`
	}
	mcpStructured(t, batch, &batchResult)
	if len(batchResult.Sets) != 3 {
		t.Fatalf("logged sets = %d, want 3", len(batchResult.Sets))
	}

	replayed := mcpToolCall(t, baseURL, token, 8, "log_sets", logArgs)
	if !reflect.DeepEqual(replayed.StructuredContent, batch.StructuredContent) {
		t.Fatalf("replayed log_sets changed the result:\nfirst: %s\nsecond: %s", batch.StructuredContent, replayed.StructuredContent)
	}

	checkIn := mcpToolCall(t, baseURL, token, 9, "check_in_session", map[string]any{
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
	mcpStructured(t, checkIn, &checkInResult)
	if !checkInResult.Session.Complete || checkInResult.Session.Revision != 2 || checkInResult.EffectiveEvidence.Source != "reported" {
		t.Fatalf("check in = %+v", checkInResult)
	}

	weekAfter := mcpToolCall(t, baseURL, token, 11, "get_week", map[string]any{"date": "2026-12-02"})
	var weekAfterResult struct {
		Performed []struct {
			ID       string `json:"id"`
			Complete bool   `json:"complete"`
			Evidence *struct {
				Source string `json:"source"`
			} `json:"evidence"`
		} `json:"performed"`
		Planned []struct {
			TemplateID string `json:"template_id"`
		} `json:"planned"`
	}
	mcpStructured(t, weekAfter, &weekAfterResult)
	if len(weekAfterResult.Performed) != 1 || weekAfterResult.Performed[0].ID != "session-a" ||
		!weekAfterResult.Performed[0].Complete || weekAfterResult.Performed[0].Evidence == nil ||
		weekAfterResult.Performed[0].Evidence.Source != "reported" {
		t.Fatalf("week performed = %+v", weekAfterResult.Performed)
	}
	if len(weekAfterResult.Planned) == 0 {
		t.Fatalf("week planned went empty: %+v", weekAfterResult)
	}

	progression := mcpToolCall(t, baseURL, token, 10, "get_progression", map[string]any{"exercise_id": "goblet-squat"})
	var progressionResult struct {
		PerformedHistory []struct {
			KG float64 `json:"kg"`
		} `json:"performed_history"`
		CurrentTargets []struct {
			TemplateID string `json:"template_id"`
		} `json:"current_targets"`
	}
	mcpStructured(t, progression, &progressionResult)
	if len(progressionResult.PerformedHistory) != 3 || len(progressionResult.CurrentTargets) != 1 {
		t.Fatalf("progression = %+v", progressionResult)
	}

	mcpToolCall(t, baseURL, token, 11, "upsert_day", map[string]any{
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

	status, body = request(t, baseURL+"/api/week/2026-12-02", token)
	if status != http.StatusOK {
		t.Fatalf("HTTP week = %d %s, want 200", status, body)
	}
	var httpWeek weekResponse
	if err := json.Unmarshal([]byte(body), &httpWeek); err != nil {
		t.Fatalf("decode HTTP week %q: %v", body, err)
	}
	if len(httpWeek.Planned) != 2 || httpWeek.Planned[0].TemplateID != "strength-template" || httpWeek.Planned[1].TemplateID != "intervals-template" {
		t.Fatalf("HTTP week planned = %+v", httpWeek.Planned)
	}
	if len(httpWeek.Days) != 1 || httpWeek.Days[0].Date != "2026-12-02" || httpWeek.Days[0].Steps == nil || *httpWeek.Days[0].Steps != 8000 {
		t.Fatalf("HTTP week days = %+v", httpWeek.Days)
	}

	process.stop(t)

	database := openDatabase(t, dbPath)
	defer database.Close()
	var batches int
	if err := database.QueryRow(`SELECT COUNT(*) FROM logged_set WHERE session_id = 'session-a'`).Scan(&batches); err != nil {
		t.Fatalf("count logged sets: %v", err)
	}
	if batches != 3 {
		t.Fatalf("logged_set rows = %d, want 3 (one batch)", batches)
	}
}
