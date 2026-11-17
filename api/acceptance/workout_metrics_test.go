package acceptance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const metricsDayBody = `{
	"date": "2026-11-10",
	"timezone": "Europe/Madrid",
	"expected_revision": %d,
	"source": "health_connect",
	"workouts": [{
		"origin": "health_connect",
		"external_id": "watch-run-1",
		"device_kind": "watch",
		"started_at": 1762758000000,
		"ended_at": 1762759740000,
		"discipline": "run",
		"duration_ms": 1740000,
		"distance_m": 5230.0,
		"avg_hr_bpm": 142,
		"max_hr_bpm": 171,
		"calories_kcal": 412.5,
		"cadence_spm": 168.0,
		"elevation_gain_m": 96.0,
		"watch_verified": true,
		"splits": [
			{"position": 0, "kind": "km", "distance_m": 1000.0, "duration_ms": 330000, "avg_hr_bpm": 138},
			{"position": 1, "kind": "km", "distance_m": 1000.0, "duration_ms": 336000, "avg_hr_bpm": 145}
		]
	}]
}`

func TestWorkoutKeepsTheWatchMetrics(t *testing.T) {
	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	status, response := postDay(t, baseURL, token, "metrics-1", fmt.Sprintf(metricsDayBody, 0))
	if status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, response)
	}
	if !strings.Contains(response, `"avg_hr_bpm":142`) {
		t.Fatalf("response does not carry avg_hr_bpm: %s", response)
	}

	status, response = request(t, baseURL+"/api/day/2026-11-10", token)
	if status != http.StatusOK {
		t.Fatalf("get day = %d %s, want 200", status, response)
	}
	day := decodeDay(t, response).Day
	if len(day.Workouts) != 1 {
		t.Fatalf("workouts = %+v", day.Workouts)
	}
	workout := day.Workouts[0]
	assertFloat(t, "avg_hr_bpm", float64Ptr(workout.AvgHRBPM), 142)
	assertFloat(t, "max_hr_bpm", float64Ptr(workout.MaxHRBPM), 171)
	assertFloat(t, "calories_kcal", workout.CaloriesKcal, 412.5)
	assertFloat(t, "cadence_spm", workout.CadenceSPM, 168)
	assertFloat(t, "elevation_gain_m", workout.ElevationGainM, 96)
	assertFloat(t, "distance_m", workout.DistanceM, 5230)
	// 1740 s over 5.23 km, derived by the server.
	assertFloat(t, "pace_sec_per_km", workout.PaceSecPerKM, 332.7)

	if len(workout.Splits) != 2 {
		t.Fatalf("splits = %+v", workout.Splits)
	}
	if workout.Splits[0].Position != 0 || workout.Splits[1].Position != 1 {
		t.Fatalf("split positions = %d/%d", workout.Splits[0].Position, workout.Splits[1].Position)
	}
	if workout.Splits[0].Kind == nil || *workout.Splits[0].Kind != "km" {
		t.Fatalf("split kind = %v", workout.Splits[0].Kind)
	}
	assertFloat(t, "split duration_ms", float64Ptr(workout.Splits[1].DurationMS), 336000)
	assertFloat(t, "split avg_hr_bpm", float64Ptr(workout.Splits[0].AvgHRBPM), 138)
	assertFloat(t, "split pace_sec_per_km", workout.Splits[0].PaceSecPerKM, 330)

	weekBody := getWeek(t, baseURL, token, "2026-11-10")
	if !strings.Contains(weekBody, `"avg_hr_bpm":142`) {
		t.Fatalf("week does not carry the workout metrics: %s", weekBody)
	}
	if !strings.Contains(weekBody, `"pace_sec_per_km":332.7`) {
		t.Fatalf("week does not carry the derived pace: %s", weekBody)
	}
}

func TestWorkoutMetricsStayAbsentInsteadOfZero(t *testing.T) {
	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	body := `{
		"date": "2026-11-11",
		"timezone": "Europe/Madrid",
		"expected_revision": 0,
		"source": "manual",
		"workouts": [{
			"origin": "manual",
			"external_id": "note-1",
			"started_at": 1762844400000,
			"ended_at": 1762846200000,
			"discipline": "run",
			"duration_ms": 1800000
		}]
	}`
	status, response := postDay(t, baseURL, token, "absent-1", body)
	if status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, response)
	}
	for _, field := range []string{"avg_hr_bpm", "max_hr_bpm", "calories_kcal", "cadence_spm", "elevation_gain_m", "pace_sec_per_km", "distance_m"} {
		if !strings.Contains(response, `"`+field+`":null`) {
			t.Fatalf("%s is not null in %s", field, response)
		}
	}

	status, response = request(t, baseURL+"/api/day/2026-11-11", token)
	if status != http.StatusOK {
		t.Fatalf("get day = %d %s, want 200", status, response)
	}
	stored := decodeDay(t, response).Day
	if len(stored.Workouts) != 1 {
		t.Fatalf("workouts = %+v", stored.Workouts)
	}
	workout := stored.Workouts[0]
	if workout.AvgHRBPM != nil || workout.MaxHRBPM != nil || workout.CaloriesKcal != nil || workout.CadenceSPM != nil || workout.ElevationGainM != nil || workout.PaceSecPerKM != nil {
		t.Fatalf("absent metrics came back as numbers: %+v", workout)
	}
	if len(workout.Splits) != 0 {
		t.Fatalf("splits = %+v, want none", workout.Splits)
	}
}

func TestWorkoutSplitsFollowTheDayWrite(t *testing.T) {
	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	if status, response := postDay(t, baseURL, token, "splits-1", fmt.Sprintf(metricsDayBody, 0)); status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, response)
	}

	oneSplit := strings.Replace(metricsDayBody, `{"position": 1, "kind": "km", "distance_m": 1000.0, "duration_ms": 336000, "avg_hr_bpm": 145}`, `{"position": 1, "kind": "km", "distance_m": 1000.0, "duration_ms": 336000, "avg_hr_bpm": 145, "pace_sec_per_km": 336}`, 1)
	oneSplit = strings.Replace(oneSplit, `"expected_revision": %d`, `"expected_revision": 1`, 1)
	status, response := postDay(t, baseURL, token, "splits-2", oneSplit)
	if status != http.StatusOK {
		t.Fatalf("update day = %d %s, want 200", status, response)
	}

	status, response = request(t, baseURL+"/api/day/2026-11-10", token)
	if status != http.StatusOK {
		t.Fatalf("get day = %d %s, want 200", status, response)
	}
	workout := decodeDay(t, response).Day.Workouts[0]
	if len(workout.Splits) != 2 {
		t.Fatalf("splits = %+v", workout.Splits)
	}
	assertFloat(t, "explicit split pace", workout.Splits[1].PaceSecPerKM, 336)

	status, response = postDay(t, baseURL, token, "splits-3", `{
		"date": "2026-11-10",
		"timezone": "Europe/Madrid",
		"expected_revision": 2,
		"source": "health_connect",
		"workouts": []
	}`)
	if status != http.StatusOK {
		t.Fatalf("clear workouts = %d %s, want 200", status, response)
	}
	status, response = request(t, baseURL+"/api/day/2026-11-10", token)
	if status != http.StatusOK {
		t.Fatalf("get day = %d %s, want 200", status, response)
	}
	if workouts := decodeDay(t, response).Day.Workouts; len(workouts) != 0 {
		t.Fatalf("workouts = %+v, want none after a write that omits them", workouts)
	}
}

func TestWorkoutRejectsImpossibleHeartRate(t *testing.T) {
	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	body := strings.Replace(metricsDayBody, `"max_hr_bpm": 171`, `"max_hr_bpm": 120`, 1)
	status, response := postDay(t, baseURL, token, "hr-1", fmt.Sprintf(body, 0))
	if status != http.StatusBadRequest {
		t.Fatalf("avg above max = %d %s, want 400", status, response)
	}
	requireErrorShape(t, response, "invalid_input")
}

func getWeek(t *testing.T, baseURL, token, date string) string {
	t.Helper()

	status, response := request(t, baseURL+"/api/week/"+date, token)
	if status != http.StatusOK {
		t.Fatalf("get week = %d %s, want 200", status, response)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(response), &decoded); err != nil {
		t.Fatalf("decode week %q: %v", response, err)
	}
	return response
}

func assertFloat(t *testing.T, field string, value *float64, want float64) {
	t.Helper()

	if value == nil {
		t.Fatalf("%s = null, want %v", field, want)
	}
	if difference := *value - want; difference > 0.05 || difference < -0.05 {
		t.Fatalf("%s = %v, want %v", field, *value, want)
	}
}

func float64Ptr(value *int64) *float64 {
	if value == nil {
		return nil
	}
	floating := float64(*value)
	return &floating
}
