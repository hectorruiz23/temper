package acceptance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type correctionEnvelope struct {
	Set          trainingSetPayload `json:"set"`
	CorrectionID string             `json:"correction_id"`
	Echo         struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Reason           string `json:"reason"`
		Changes          struct {
			ExerciseID *string  `json:"exercise_id"`
			SetIndex   *int     `json:"set_index"`
			KG         *float64 `json:"kg"`
			Reps       *int     `json:"reps"`
			Note       *string  `json:"note"`
		} `json:"changes"`
	} `json:"echo"`
}

type progressionEnvelope struct {
	PerformedHistory []performedSetPayload `json:"performed_history"`
	CurrentTargets   []struct {
		TemplateID string   `json:"template_id"`
		TargetSets *int     `json:"target_sets"`
		RepsMin    *int     `json:"reps_min"`
		RepsMax    *int     `json:"reps_max"`
		TargetKG   *float64 `json:"target_kg"`
		Tempo      *string  `json:"tempo"`
	} `json:"current_targets"`
	ProgressionDimensions []struct {
		Name      string          `json:"name"`
		Dimension json.RawMessage `json:"dimension"`
		Basis     string          `json:"basis"`
	} `json:"progression_dimensions"`
}

type performedSetPayload struct {
	SetID          string  `json:"set_id"`
	SessionID      string  `json:"session_id"`
	Date           string  `json:"date"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	Revision       int64   `json:"revision"`
}

func decodeCorrection(t *testing.T, body string) correctionEnvelope {
	t.Helper()

	var envelope correctionEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode correction %q: %v", body, err)
	}
	return envelope
}

func decodeProgression(t *testing.T, body string) progressionEnvelope {
	t.Helper()

	var envelope progressionEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode progression %q: %v", body, err)
	}
	return envelope
}

func TestSetCorrectionsAndProgression(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "temper.db")
	templateID := seedTrainingPlan(t, dbPath)
	token := randomToken(t)

	process := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL := process.waitListening(t, 15*time.Second)

	createBody := fmt.Sprintf(`{"id":"session-a","template_id":%q,"date":"2026-12-02","discipline":"strength"}`, templateID)
	status, body := postTrainingJSON(t, baseURL+"/api/sessions", token, "create-a", createBody)
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %s, want 201", status, body)
	}

	batchBody := `{"session_id":"session-a","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20,"reps":10},{"exercise_id":"goblet-squat","set_index":2,"kg":20,"reps":8}]}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets", token, "batch-a", batchBody)
	if status != http.StatusCreated {
		t.Fatalf("log sets = %d %s, want 201", status, body)
	}
	batch := decodeTrainingBatch(t, body)
	if len(batch.Sets) != 2 {
		t.Fatalf("logged sets = %+v", batch.Sets)
	}
	firstSetID := batch.Sets[0].ID

	correctionBody := `{"expected_revision":1,"reason":"  adjusted reps  ","changes":{"reps":12,"note":"  stronger  "}}`
	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-1", correctionBody)
	if status != http.StatusOK {
		t.Fatalf("correct set = %d %s, want 200", status, body)
	}
	corrected := decodeCorrection(t, body)
	if corrected.Set.ID != firstSetID || corrected.Set.Revision != 2 || corrected.Set.Reps != 12 {
		t.Fatalf("corrected set = %+v", corrected.Set)
	}
	if corrected.Set.Note == nil || *corrected.Set.Note != "stronger" {
		t.Fatalf("corrected note = %v", corrected.Set.Note)
	}
	if corrected.CorrectionID == "" {
		t.Fatal("correction_id is empty")
	}
	if corrected.Echo.ExpectedRevision != 1 || corrected.Echo.Reason != "adjusted reps" || corrected.Echo.Changes.Reps == nil || *corrected.Echo.Changes.Reps != 12 {
		t.Fatalf("echo = %+v", corrected.Echo)
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-1", correctionBody)
	if status != http.StatusOK {
		t.Fatalf("replay correction = %d %s, want 200", status, body)
	}
	if replayed := decodeCorrection(t, body); !reflect.DeepEqual(replayed, corrected) {
		t.Fatalf("replay = %+v, want %+v", replayed, corrected)
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-stale", `{"expected_revision":1,"reason":"stale","changes":{"reps":11}}`)
	if status != http.StatusConflict {
		t.Fatalf("stale correction = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "revision_conflict")

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-limit", `{"expected_revision":2,"reason":"too heavy","changes":{"kg":25}}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("equipment limit = %d %s, want 422", status, body)
	}
	requireErrorShape(t, body, "equipment_limit")

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-position", `{"expected_revision":2,"reason":"move","changes":{"set_index":2}}`)
	if status != http.StatusConflict {
		t.Fatalf("occupied position = %d %s, want 409", status, body)
	}
	requireErrorShape(t, body, "set_position_occupied")

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-noop", `{"expected_revision":2,"reason":"restated","changes":{"kg":20}}`)
	if status != http.StatusOK {
		t.Fatalf("no-op correction = %d %s, want 200", status, body)
	}
	noop := decodeCorrection(t, body)
	if noop.Set.Revision != 3 || noop.CorrectionID == corrected.CorrectionID {
		t.Fatalf("no-op correction = %+v", noop)
	}

	status, body = postTrainingJSON(t, baseURL+"/api/sets/missing/correct", token, "correct-missing", `{"expected_revision":1,"reason":"missing","changes":{"reps":11}}`)
	if status != http.StatusNotFound {
		t.Fatalf("missing set = %d %s, want 404", status, body)
	}
	requireErrorShape(t, body, "not_found")

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-empty-reason", `{"expected_revision":3,"reason":"   ","changes":{"reps":11}}`)
	if status != http.StatusBadRequest {
		t.Fatalf("empty reason = %d %s, want 400", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	status, body = postTrainingJSON(t, baseURL+"/api/sets/"+firstSetID+"/correct", token, "correct-no-changes", `{"expected_revision":3,"reason":"nothing","changes":{}}`)
	if status != http.StatusBadRequest {
		t.Fatalf("no changes = %d %s, want 400", status, body)
	}
	requireErrorShape(t, body, "invalid_input")

	status, body = request(t, baseURL+"/api/progression/goblet-squat", token)
	if status != http.StatusOK {
		t.Fatalf("progression = %d %s, want 200", status, body)
	}
	progression := decodeProgression(t, body)
	if len(progression.PerformedHistory) != 2 {
		t.Fatalf("history = %+v", progression.PerformedHistory)
	}
	var historySet *performedSetPayload
	for index := range progression.PerformedHistory {
		if progression.PerformedHistory[index].SetID == firstSetID {
			historySet = &progression.PerformedHistory[index]
		}
	}
	if historySet == nil {
		t.Fatalf("corrected set missing from history: %+v", progression.PerformedHistory)
	}
	if historySet.Reps != 12 || historySet.Revision != 3 || historySet.KG != 20 || historySet.SessionID != "session-a" || historySet.Date != "2026-12-02" {
		t.Fatalf("history set = %+v", historySet)
	}
	if historySet.Note == nil || *historySet.Note != "stronger" {
		t.Fatalf("history note = %v", historySet.Note)
	}

	if len(progression.CurrentTargets) != 1 {
		t.Fatalf("current targets = %+v", progression.CurrentTargets)
	}
	target := progression.CurrentTargets[0]
	if target.TemplateID != templateID || target.TargetSets == nil || *target.TargetSets != 3 {
		t.Fatalf("target = %+v", target)
	}
	if target.RepsMin == nil || *target.RepsMin != 8 || target.RepsMax == nil || *target.RepsMax != 12 {
		t.Fatalf("target reps = %+v", target)
	}
	if target.TargetKG == nil || *target.TargetKG != 20 {
		t.Fatalf("target kg = %v", target.TargetKG)
	}

	dimensions := make(map[string]string, len(progression.ProgressionDimensions))
	for _, dimension := range progression.ProgressionDimensions {
		dimensions[dimension.Name] = string(dimension.Dimension)
		if dimension.Basis != "session_exercise" {
			t.Fatalf("dimension %q basis = %q", dimension.Name, dimension.Basis)
		}
	}
	if dimensions["sets"] != "3" {
		t.Fatalf("sets dimension = %q", dimensions["sets"])
	}
	if dimensions["reps"] != `"8-12"` {
		t.Fatalf("reps dimension = %q", dimensions["reps"])
	}

	status, body = request(t, baseURL+"/api/progression/missing", token)
	if status != http.StatusNotFound {
		t.Fatalf("missing exercise = %d %s, want 404", status, body)
	}
	requireErrorShape(t, body, "not_found")

	status, body = postTrainingJSON(t, baseURL+"/api/exercises", token, "exercise-unilateral", `{"name":"Single leg curl","movement_pattern":"hinge","unilateral":true,"equipment":[]}`)
	if status != http.StatusCreated {
		t.Fatalf("create unilateral exercise = %d %s, want 201", status, body)
	}
	status, body = request(t, baseURL+"/api/progression/single-leg-curl", token)
	if status != http.StatusOK {
		t.Fatalf("unilateral progression = %d %s, want 200", status, body)
	}
	unilateral := decodeProgression(t, body)
	if len(unilateral.PerformedHistory) != 0 || len(unilateral.CurrentTargets) != 0 {
		t.Fatalf("unilateral progression = %+v", unilateral)
	}
	if len(unilateral.ProgressionDimensions) != 1 || unilateral.ProgressionDimensions[0].Name != "unilateral" || string(unilateral.ProgressionDimensions[0].Dimension) != "true" || unilateral.ProgressionDimensions[0].Basis != "exercise" {
		t.Fatalf("unilateral dimensions = %+v", unilateral.ProgressionDimensions)
	}

	process.stop(t)

	database := openDatabase(t, dbPath)
	var corrections int
	if err := database.QueryRow(`SELECT COUNT(*) FROM set_correction WHERE set_id = ?`, firstSetID).Scan(&corrections); err != nil {
		t.Fatalf("count corrections: %v", err)
	}
	if corrections != 2 {
		t.Fatalf("set_correction rows = %d, want 2", corrections)
	}

	rows, err := database.Query(`SELECT before_json, after_json, resulting_revision FROM set_correction WHERE set_id = ? ORDER BY resulting_revision`, firstSetID)
	if err != nil {
		t.Fatalf("read corrections: %v", err)
	}
	defer rows.Close()
	var revisions []int64
	for rows.Next() {
		var (
			before    string
			after     string
			resulting int64
		)
		if err := rows.Scan(&before, &after, &resulting); err != nil {
			t.Fatalf("scan correction: %v", err)
		}
		revisions = append(revisions, resulting)
		switch resulting {
		case 2:
			if !strings.Contains(before, `"reps":10`) || !strings.Contains(after, `"reps":12`) {
				t.Fatalf("first correction before = %s after = %s", before, after)
			}
		case 3:
			if !strings.Contains(before, `"reps":12`) || !strings.Contains(after, `"reps":12`) {
				t.Fatalf("no-op correction before = %s after = %s", before, after)
			}
		}
	}
	if !reflect.DeepEqual(revisions, []int64{2, 3}) {
		t.Fatalf("resulting revisions = %v, want [2 3]", revisions)
	}

	var revision int64
	var reps int
	if err := database.QueryRow(`SELECT revision, reps FROM logged_set WHERE id = ?`, firstSetID).Scan(&revision, &reps); err != nil {
		t.Fatalf("read corrected set: %v", err)
	}
	if revision != 3 || reps != 12 {
		t.Fatalf("logged set revision = %d reps = %d, want 3 and 12", revision, reps)
	}
}
