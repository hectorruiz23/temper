package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type dayResponseEnvelope struct {
	Day struct {
		Date         string   `json:"date"`
		Timezone     string   `json:"timezone"`
		Revision     int64    `json:"revision"`
		SleepMS      *int64   `json:"sleep_ms"`
		Steps        *int64   `json:"steps"`
		RestingHRBPM *int64   `json:"resting_hr_bpm"`
		HRVRMSSDMS   *float64 `json:"hrv_rmssd_ms"`
		UpdatedAt    int64    `json:"updated_at"`
		LastSource   string   `json:"last_source"`
		SleepStages  []struct {
			Stage      string `json:"stage"`
			DurationMS int64  `json:"duration_ms"`
		} `json:"sleep_stages"`
		Workouts []struct {
			ID             string   `json:"id"`
			Origin         string   `json:"origin"`
			ExternalID     string   `json:"external_id"`
			DeviceKind     *string  `json:"device_kind"`
			StartedAt      int64    `json:"started_at"`
			EndedAt        int64    `json:"ended_at"`
			Discipline     string   `json:"discipline"`
			DurationMS     int64    `json:"duration_ms"`
			DistanceM      *float64 `json:"distance_m"`
			AvgHRBPM       *int64   `json:"avg_hr_bpm"`
			MaxHRBPM       *int64   `json:"max_hr_bpm"`
			CaloriesKcal   *float64 `json:"calories_kcal"`
			CadenceSPM     *float64 `json:"cadence_spm"`
			ElevationGainM *float64 `json:"elevation_gain_m"`
			PaceSecPerKM   *float64 `json:"pace_sec_per_km"`
			WatchVerified  bool     `json:"watch_verified"`
			Splits         []struct {
				Position     int      `json:"position"`
				Kind         *string  `json:"kind"`
				DistanceM    *float64 `json:"distance_m"`
				DurationMS   *int64   `json:"duration_ms"`
				AvgHRBPM     *int64   `json:"avg_hr_bpm"`
				PaceSecPerKM *float64 `json:"pace_sec_per_km"`
			} `json:"splits"`
		} `json:"workouts"`
	} `json:"day"`
}

func postDay(t *testing.T, url, token, key, body string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, url+"/api/day", strings.NewReader(body))
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
		t.Fatalf("post day: %v", err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return response.StatusCode, strings.TrimSpace(string(content))
}

func decodeDay(t *testing.T, body string) dayResponseEnvelope {
	t.Helper()

	var envelope dayResponseEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode day %q: %v", body, err)
	}
	return envelope
}

func TestHealthDayIngestAndRead(t *testing.T) {
	location, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatalf("load Europe/Madrid: %v", err)
	}

	midnight := time.Date(2026, 10, 25, 0, 0, 0, 0, location)
	nextMidnight := time.Date(2026, 10, 26, 0, 0, 0, 0, location)
	_, midnightOffset := midnight.Zone()
	_, nextMidnightOffset := nextMidnight.Zone()
	if midnightOffset == nextMidnightOffset {
		t.Fatalf("2026-10-25 does not cross a daylight-saving boundary: offsets are both %d", midnightOffset)
	}

	started := time.Date(2026, 10, 24, 23, 30, 0, 0, location)
	ended := started.Add(time.Hour)
	if started.YearDay() == ended.YearDay() {
		t.Fatalf("fixture workout is not cross-midnight: %s to %s", started, ended)
	}

	body := fmt.Sprintf(`{
		"date": "2026-10-25",
		"timezone": "Europe/Madrid",
		"expected_revision": 0,
		"source": "health_connect",
		"metrics": {"sleep_ms": 25200000, "steps": 0, "resting_hr_bpm": 52},
		"sleep_stages": [{"stage": "deep", "duration_ms": 7200000}],
		"workouts": [{
			"origin": "health_connect",
			"external_id": "abc-123",
			"device_kind": "watch",
			"started_at": %d,
			"ended_at": %d,
			"discipline": "run",
			"duration_ms": 3600000,
			"distance_m": 5200.0,
			"watch_verified": true
		}]
	}`, started.UnixMilli(), ended.UnixMilli())

	token := randomToken(t)
	process := startProcess(t, filepath.Join(t.TempDir(), "temper.db"), token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	status, response := postDay(t, baseURL, token, "create-key", body)
	if status != http.StatusOK {
		t.Fatalf("create day = %d %s, want 200", status, response)
	}
	created := decodeDay(t, response)
	if created.Day.Date != "2026-10-25" || created.Day.Timezone != "Europe/Madrid" || created.Day.Revision != 1 {
		t.Fatalf("created day = %+v", created.Day)
	}
	if created.Day.SleepMS == nil || *created.Day.SleepMS != 25200000 {
		t.Fatalf("sleep_ms = %v", created.Day.SleepMS)
	}
	if created.Day.Steps == nil || *created.Day.Steps != 0 {
		t.Fatalf("steps = %v, want an explicit zero", created.Day.Steps)
	}
	if created.Day.RestingHRBPM == nil || *created.Day.RestingHRBPM != 52 {
		t.Fatalf("resting_hr_bpm = %v", created.Day.RestingHRBPM)
	}
	if created.Day.HRVRMSSDMS != nil {
		t.Fatalf("hrv_rmssd_ms = %v, want absent", *created.Day.HRVRMSSDMS)
	}
	if created.Day.UpdatedAt <= 0 {
		t.Fatalf("updated_at = %d, want positive", created.Day.UpdatedAt)
	}
	if created.Day.LastSource != "health_connect" {
		t.Fatalf("last_source = %q", created.Day.LastSource)
	}
	if len(created.Day.SleepStages) != 1 || created.Day.SleepStages[0].Stage != "deep" {
		t.Fatalf("sleep_stages = %+v", created.Day.SleepStages)
	}
	if len(created.Day.Workouts) != 1 {
		t.Fatalf("workouts = %+v", created.Day.Workouts)
	}
	workout := created.Day.Workouts[0]
	if workout.ID == "" || workout.ExternalID != "abc-123" || !workout.WatchVerified {
		t.Fatalf("workout = %+v", workout)
	}
	if workout.StartedAt != started.UnixMilli() || workout.EndedAt != ended.UnixMilli() {
		t.Fatalf("workout times = %d/%d, want %d/%d", workout.StartedAt, workout.EndedAt, started.UnixMilli(), ended.UnixMilli())
	}
	if workout.DeviceKind == nil || *workout.DeviceKind != "watch" {
		t.Fatalf("device_kind = %v", workout.DeviceKind)
	}
	if workout.DistanceM == nil || *workout.DistanceM != 5200 {
		t.Fatalf("distance_m = %v", workout.DistanceM)
	}

	status, response = postDay(t, baseURL, token, "create-key", body)
	if status != http.StatusOK {
		t.Fatalf("idempotent replay = %d %s, want 200", status, response)
	}
	replayed := decodeDay(t, response)
	if replayed.Day.Revision != created.Day.Revision || replayed.Day.UpdatedAt != created.Day.UpdatedAt {
		t.Fatalf("replay changed the day: %+v then %+v", created.Day, replayed.Day)
	}
	if len(replayed.Day.Workouts) != 1 || replayed.Day.Workouts[0].ID != workout.ID {
		t.Fatalf("replay changed the workout: %+v", replayed.Day.Workouts)
	}

	conflicting := strings.Replace(body, `"steps": 0`, `"steps": 1`, 1)
	status, response = postDay(t, baseURL, token, "create-key", conflicting)
	if status != http.StatusConflict {
		t.Fatalf("idempotency conflict = %d %s, want 409", status, response)
	}
	requireErrorShape(t, response, "idempotency_conflict")

	stale := strings.Replace(body, `"expected_revision": 0`, `"expected_revision": 2`, 1)
	status, response = postDay(t, baseURL, token, "stale-key", stale)
	if status != http.StatusConflict {
		t.Fatalf("stale revision = %d %s, want 409", status, response)
	}
	requireErrorShape(t, response, "revision_conflict")

	status, response = request(t, baseURL+"/api/day/2026-10-25", token)
	if status != http.StatusOK {
		t.Fatalf("get day = %d %s, want 200", status, response)
	}
	stored := decodeDay(t, response)
	if stored.Day.Revision != 1 || len(stored.Day.Workouts) != 1 || stored.Day.Workouts[0].ID != workout.ID {
		t.Fatalf("stored day = %+v", stored.Day)
	}

	status, response = request(t, baseURL+"/api/day/2026-10-31", token)
	if status != http.StatusNotFound {
		t.Fatalf("missing day = %d %s, want 404", status, response)
	}
	requireErrorShape(t, response, "not_found")

	status, response = postDay(t, baseURL, token, "unknown-key", strings.Replace(body, `"date": "2026-10-25"`, `"date": "2026-10-25", "unknown": true`, 1))
	if status != http.StatusBadRequest {
		t.Fatalf("unknown field = %d %s, want 400", status, response)
	}
	requireErrorShape(t, response, "invalid_input")

	status, response = postDay(t, baseURL, token, "", body)
	if status != http.StatusBadRequest {
		t.Fatalf("missing idempotency key = %d %s, want 400", status, response)
	}
	requireErrorShape(t, response, "idempotency_key_required")

	oversized := `{"date":"2026-11-17","timezone":"` + strings.Repeat("a", 64<<10) + `","source":"manual"}`
	status, response = postDay(t, baseURL, token, "large-key", oversized)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d %s, want 413", status, response)
	}
	requireErrorShape(t, response, "body_too_large")
}
