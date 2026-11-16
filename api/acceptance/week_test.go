package acceptance

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type weekPlanEnvelope struct {
	Plan struct {
		ID              int64  `json:"id"`
		CreatedAt       string `json:"created_at"`
		Reason          string `json:"reason"`
		EffectiveFrom   string `json:"effective_from"`
		Timezone        string `json:"timezone"`
		WeeklyFrequency int    `json:"weekly_frequency"`
		Sessions        []struct {
			ID            string  `json:"id"`
			Position      int     `json:"position"`
			ScheduledDate *string `json:"scheduled_date"`
			Weekday       *int    `json:"weekday"`
			Discipline    string  `json:"discipline"`
			Target        string  `json:"target"`
			Exercises     []struct {
				Position   int      `json:"position"`
				ExerciseID string   `json:"exercise_id"`
				TargetSets *int     `json:"target_sets"`
				RepsMin    *int     `json:"reps_min"`
				RepsMax    *int     `json:"reps_max"`
				TargetKG   *float64 `json:"target_kg"`
				Tempo      *string  `json:"tempo"`
			} `json:"exercises"`
			IntervalSteps []struct {
				Position   int      `json:"position"`
				Label      string   `json:"label"`
				DurationMS *int     `json:"duration_ms"`
				DistanceM  *float64 `json:"distance_m"`
			} `json:"interval_steps"`
		} `json:"sessions"`
	} `json:"plan"`
}

type weekExerciseEnvelope struct {
	Exercise struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		MovementPattern string `json:"movement_pattern"`
		Unilateral      bool   `json:"unilateral"`
		Equipment       []struct {
			EquipmentID string `json:"equipment_id"`
			Quantity    int    `json:"quantity"`
		} `json:"equipment"`
	} `json:"exercise"`
}

type weekResponse struct {
	WeekStart string `json:"week_start"`
	WeekEnd   string `json:"week_end"`
	Planned   []struct {
		Date          string `json:"date"`
		TemplateID    string `json:"template_id"`
		Discipline    string `json:"discipline"`
		Target        string `json:"target"`
		PlanVersionID int64  `json:"plan_version_id"`
	} `json:"planned"`
	Performed []json.RawMessage `json:"performed"`
	Days      []struct {
		Date         string   `json:"date"`
		Revision     int64    `json:"revision"`
		SleepMS      *int64   `json:"sleep_ms"`
		Steps        *int64   `json:"steps"`
		RestingHRBPM *int64   `json:"resting_hr_bpm"`
		HRVRMSSDMS   *float64 `json:"hrv_rmssd_ms"`
	} `json:"days"`
}

func weekPost(t *testing.T, url, token, key, body string) (int, string) {
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
		t.Fatalf("read response: %v", err)
	}

	return response.StatusCode, strings.TrimSpace(string(content))
}

func decodeWeek(t *testing.T, body string) weekResponse {
	t.Helper()

	var response weekResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode week %q: %v", body, err)
	}
	return response
}

func weekPlanBody(effectiveFrom, suffix string) string {
	return `{
		"reason":"week block ` + suffix + `",
		"effective_from":"` + effectiveFrom + `",
		"timezone":"Europe/Madrid",
		"weekly_frequency":3,
		"sessions":[
			{
				"id":"strength-` + suffix + `",
				"position":0,
				"scheduled_date":"` + weekStrengthDate(suffix) + `",
				"discipline":"strength",
				"target":"full body",
				"exercises":[
					{"position":0,"exercise_id":"goblet-squat","target_sets":3,"reps_min":8,"reps_max":12,"target_kg":20,"tempo":"3-1-1"}
				]
			},
			{
				"id":"weekly-` + suffix + `",
				"position":1,
				"weekday":` + weekWeeklyDay(suffix) + `,
				"discipline":"intervals",
				"target":"repetitions",
				"interval_steps":[
					{"position":0,"label":"warm up","duration_ms":600000},
					{"position":1,"label":"repetition","distance_m":400}
				]
			}
		]
	}`
}

func weekStrengthDate(suffix string) string {
	if suffix == "v1" {
		return "2026-12-30"
	}
	return "2027-01-06"
}

func weekWeeklyDay(suffix string) string {
	if suffix == "v1" {
		return "6"
	}
	return "2"
}

func TestWeekViewOverHTTP(t *testing.T) {
	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	exerciseBody := `{"name":"Goblet squat","movement_pattern":"squat","unilateral":false,"equipment":[{"equipment_id":"adjustable-dumbbell","quantity":1}]}`
	status, response := weekPost(t, baseURL+"/api/exercises", token, "exercise-1", exerciseBody)
	if status != http.StatusCreated {
		t.Fatalf("create exercise = %d %s, want 201", status, response)
	}
	var exercise weekExerciseEnvelope
	if err := json.Unmarshal([]byte(response), &exercise); err != nil {
		t.Fatalf("decode exercise: %v", err)
	}
	if exercise.Exercise.ID != "goblet-squat" || len(exercise.Exercise.Equipment) != 1 {
		t.Fatalf("created exercise = %+v", exercise.Exercise)
	}

	status, response = weekPost(t, baseURL+"/api/plans", token, "plan-v1", weekPlanBody("2026-12-01", "v1"))
	if status != http.StatusCreated {
		t.Fatalf("create plan v1 = %d %s, want 201", status, response)
	}
	var first weekPlanEnvelope
	if err := json.Unmarshal([]byte(response), &first); err != nil {
		t.Fatalf("decode plan v1: %v", err)
	}
	if first.Plan.ID <= 0 || first.Plan.EffectiveFrom != "2026-12-01" || len(first.Plan.Sessions) != 2 {
		t.Fatalf("plan v1 = %+v", first.Plan)
	}
	if _, err := time.Parse(time.RFC3339, first.Plan.CreatedAt); err != nil {
		t.Fatalf("created_at %q is not RFC 3339: %v", first.Plan.CreatedAt, err)
	}
	if len(first.Plan.Sessions[1].IntervalSteps) != 2 {
		t.Fatalf("plan v1 interval steps = %+v", first.Plan.Sessions[1].IntervalSteps)
	}

	status, response = weekPost(t, baseURL+"/api/plans", token, "plan-v2", weekPlanBody("2027-01-04", "v2"))
	if status != http.StatusCreated {
		t.Fatalf("create plan v2 = %d %s, want 201", status, response)
	}
	var second weekPlanEnvelope
	if err := json.Unmarshal([]byte(response), &second); err != nil {
		t.Fatalf("decode plan v2: %v", err)
	}
	if second.Plan.ID <= 0 || second.Plan.ID == first.Plan.ID {
		t.Fatalf("plan v2 = %+v", second.Plan)
	}

	status, response = request(t, baseURL+"/api/plans/current", token)
	if status != http.StatusOK {
		t.Fatalf("current plan = %d %s, want 200", status, response)
	}
	var current weekPlanEnvelope
	if err := json.Unmarshal([]byte(response), &current); err != nil {
		t.Fatalf("decode current plan: %v", err)
	}
	if current.Plan.ID != second.Plan.ID {
		t.Fatalf("current plan id = %d, want %d", current.Plan.ID, second.Plan.ID)
	}

	status, response = request(t, baseURL+"/api/plans/"+weekInt(first.Plan.ID), token)
	if status != http.StatusOK {
		t.Fatalf("plan by id = %d %s, want 200", status, response)
	}
	var byID weekPlanEnvelope
	if err := json.Unmarshal([]byte(response), &byID); err != nil {
		t.Fatalf("decode plan by id: %v", err)
	}
	if byID.Plan.ID != first.Plan.ID || len(byID.Plan.Sessions) != 2 {
		t.Fatalf("plan by id = %+v", byID.Plan)
	}

	dayBody := `{"date":"2026-12-30","timezone":"Europe/Madrid","expected_revision":0,"source":"manual","metrics":{"sleep_ms":25200000,"steps":8000,"resting_hr_bpm":52}}`
	status, response = weekPost(t, baseURL+"/api/day", token, "day-1", dayBody)
	if status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, response)
	}

	status, response = request(t, baseURL+"/api/week/2026-12-31", token)
	if status != http.StatusOK {
		t.Fatalf("week across year boundary = %d %s, want 200", status, response)
	}
	crossYear := decodeWeek(t, response)
	if crossYear.WeekStart != "2026-12-28" || crossYear.WeekEnd != "2027-01-03" {
		t.Fatalf("cross-year bounds = %s..%s", crossYear.WeekStart, crossYear.WeekEnd)
	}
	if len(crossYear.Planned) != 2 {
		t.Fatalf("cross-year planned = %+v", crossYear.Planned)
	}
	if crossYear.Planned[0].Date != "2026-12-30" || crossYear.Planned[0].TemplateID != "strength-v1" || crossYear.Planned[0].PlanVersionID != first.Plan.ID {
		t.Fatalf("cross-year dated entry = %+v", crossYear.Planned[0])
	}
	if crossYear.Planned[1].Date != "2027-01-02" || crossYear.Planned[1].TemplateID != "weekly-v1" || crossYear.Planned[1].Discipline != "intervals" {
		t.Fatalf("cross-year weekly entry = %+v", crossYear.Planned[1])
	}
	if len(crossYear.Performed) != 0 {
		t.Fatalf("performed = %+v, want empty", crossYear.Performed)
	}
	if len(crossYear.Days) != 1 {
		t.Fatalf("days = %+v", crossYear.Days)
	}
	day := crossYear.Days[0]
	if day.Date != "2026-12-30" || day.Revision != 1 || day.SleepMS == nil || *day.SleepMS != 25200000 || day.Steps == nil || *day.Steps != 8000 || day.RestingHRBPM == nil || *day.RestingHRBPM != 52 || day.HRVRMSSDMS != nil {
		t.Fatalf("week day = %+v", day)
	}

	status, response = request(t, baseURL+"/api/week/2027-01-08", token)
	if status != http.StatusOK {
		t.Fatalf("week in second version = %d %s, want 200", status, response)
	}
	secondWeek := decodeWeek(t, response)
	if secondWeek.WeekStart != "2027-01-04" || secondWeek.WeekEnd != "2027-01-10" {
		t.Fatalf("second week bounds = %s..%s", secondWeek.WeekStart, secondWeek.WeekEnd)
	}
	if len(secondWeek.Planned) != 2 {
		t.Fatalf("second week planned = %+v", secondWeek.Planned)
	}
	if secondWeek.Planned[0].Date != "2027-01-05" || secondWeek.Planned[0].PlanVersionID != second.Plan.ID {
		t.Fatalf("second week weekly entry = %+v", secondWeek.Planned[0])
	}
	if secondWeek.Planned[1].Date != "2027-01-06" || secondWeek.Planned[1].PlanVersionID != second.Plan.ID {
		t.Fatalf("second week dated entry = %+v", secondWeek.Planned[1])
	}
	if len(secondWeek.Performed) != 0 {
		t.Fatalf("second week performed = %+v, want empty", secondWeek.Performed)
	}
	if len(secondWeek.Days) != 0 {
		t.Fatalf("second week days = %+v, want empty", secondWeek.Days)
	}

	status, response = request(t, baseURL+"/api/week/2026-13-01", token)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid week date = %d %s, want 400", status, response)
	}
	requireErrorShape(t, response, "invalid_input")

	status, response = request(t, baseURL+"/api/week/2026-12-31", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("week without token = %d %s, want 401", status, response)
	}
	requireErrorShape(t, response, "unauthorized")
}

func weekInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
