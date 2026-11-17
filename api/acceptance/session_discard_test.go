package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

type discardWeekEnvelope struct {
	Performed []struct {
		ID   string `json:"id"`
		Date string `json:"date"`
	} `json:"performed"`
}

func discardRequest(t *testing.T, url, token string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	request.Header.Set("X-Temper-Token", token)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("delete %s: %v", url, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read delete response: %v", err)
	}
	return response.StatusCode, string(body)
}

func discardPerformed(t *testing.T, baseURL, token, date string) []string {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/week/"+date, nil)
	if err != nil {
		t.Fatalf("build week request: %v", err)
	}
	request.Header.Set("X-Temper-Token", token)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("week %s: %v", date, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("week %s = %d", date, response.StatusCode)
	}

	var decoded discardWeekEnvelope
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode week: %v", err)
	}

	ids := make([]string, 0, len(decoded.Performed))
	for _, session := range decoded.Performed {
		ids = append(ids, session.ID)
	}
	return ids
}

func TestSessionWithoutSetsCanBeDiscarded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "temper.db")
	templateID := seedTrainingPlan(t, dbPath)
	token := randomToken(t)

	process := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	emptyBody := fmt.Sprintf(`{"id":"discard-empty","template_id":%q,"date":"2026-12-02","discipline":"strength","started_at":"2026-12-02T08:00:00Z"}`, templateID)
	status, body := postTrainingJSON(t, baseURL+"/api/sessions", token, "discard-create-empty", emptyBody)
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %s, want 201", status, body)
	}

	if ids := discardPerformed(t, baseURL, token, "2026-12-02"); len(ids) != 1 || ids[0] != "discard-empty" {
		t.Fatalf("performed before discard = %v, want [discard-empty]", ids)
	}

	status, body = discardRequest(t, baseURL+"/api/sessions/discard-empty", token)
	if status != http.StatusNoContent {
		t.Fatalf("discard = %d %s, want 204", status, body)
	}
	if body != "" {
		t.Fatalf("discard body = %q, want empty", body)
	}

	if ids := discardPerformed(t, baseURL, token, "2026-12-02"); len(ids) != 0 {
		t.Fatalf("performed after discard = %v, want none", ids)
	}

	status, body = discardRequest(t, baseURL+"/api/sessions/discard-empty", token)
	if status != http.StatusNotFound {
		t.Fatalf("discard again = %d %s, want 404", status, body)
	}
	requireErrorShape(t, body, "not_found")

	withSetsBody := fmt.Sprintf(`{"id":"discard-with-sets","template_id":%q,"date":"2026-12-03","discipline":"strength","started_at":"2026-12-03T08:00:00Z"}`, templateID)
	status, body = postTrainingJSON(t, baseURL+"/api/sessions", token, "discard-create-sets", withSetsBody)
	if status != http.StatusCreated {
		t.Fatalf("create session with sets = %d %s, want 201", status, body)
	}

	batchBody := `{"session_id":"discard-with-sets","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20,"reps":10}]}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "discard-batch", batchBody)
	if status != http.StatusCreated {
		t.Fatalf("log sets = %d %s, want 201", status, body)
	}

	status, body = discardRequest(t, baseURL+"/api/sessions/discard-with-sets", token)
	if status != http.StatusConflict {
		t.Fatalf("discard with sets = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "session_has_sets")

	if ids := discardPerformed(t, baseURL, token, "2026-12-03"); len(ids) != 1 || ids[0] != "discard-with-sets" {
		t.Fatalf("performed after refused discard = %v, want the session kept", ids)
	}

	status, body = discardRequest(t, baseURL+"/api/sessions/does-not-exist", token)
	if status != http.StatusNotFound {
		t.Fatalf("discard unknown = %d %s, want 404", status, body)
	}
}
