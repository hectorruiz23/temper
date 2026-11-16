package domain

import (
	"math"
	"strings"
	"testing"
)

func correctionIntPtr(value int) *int {
	return &value
}

func correctionFloatPtr(value float64) *float64 {
	return &value
}

func correctionStringPtr(value string) *string {
	return &value
}

func correctionFixture() Set {
	return Set{
		ID:             "set-1",
		SessionID:      "session-1",
		ExerciseID:     "goblet-squat",
		SetIndex:       1,
		KG:             20,
		Reps:           10,
		EvidenceSource: EvidenceReported,
		IdempotencyKey: correctionStringPtr("key-1"),
		CreatedAt:      1000,
		Revision:       1,
	}
}

func TestNormalizeCorrection(t *testing.T) {
	reason, changes, err := NormalizeCorrection("  adjusted reps  ", SetChanges{
		ExerciseID: correctionStringPtr("  goblet-squat  "),
		SetIndex:   correctionIntPtr(2),
		KG:         correctionFloatPtr(20.005),
		Reps:       correctionIntPtr(12),
		Note:       correctionStringPtr("  stronger  "),
	})
	if err != nil {
		t.Fatalf("normalize correction: %v", err)
	}
	if reason != "adjusted reps" {
		t.Fatalf("reason = %q", reason)
	}
	if changes.ExerciseID == nil || *changes.ExerciseID != "goblet-squat" {
		t.Fatalf("exercise id = %v", changes.ExerciseID)
	}
	if changes.SetIndex == nil || *changes.SetIndex != 2 {
		t.Fatalf("set index = %v", changes.SetIndex)
	}
	if changes.KG == nil || *changes.KG != 20.01 {
		t.Fatalf("kg = %v", changes.KG)
	}
	if changes.Reps == nil || *changes.Reps != 12 {
		t.Fatalf("reps = %v", changes.Reps)
	}
	if changes.Note == nil || *changes.Note != "stronger" {
		t.Fatalf("note = %v", changes.Note)
	}
}

func TestNormalizeCorrectionRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		changes SetChanges
	}{
		{name: "empty reason", reason: "   ", changes: SetChanges{Reps: correctionIntPtr(10)}},
		{name: "no changes", reason: "fix", changes: SetChanges{}},
		{name: "empty exercise", reason: "fix", changes: SetChanges{ExerciseID: correctionStringPtr("  ")}},
		{name: "zero position", reason: "fix", changes: SetChanges{SetIndex: correctionIntPtr(0)}},
		{name: "negative kg", reason: "fix", changes: SetChanges{KG: correctionFloatPtr(-1)}},
		{name: "nan kg", reason: "fix", changes: SetChanges{KG: correctionFloatPtr(math.NaN())}},
		{name: "infinite kg", reason: "fix", changes: SetChanges{KG: correctionFloatPtr(math.Inf(1))}},
		{name: "zero reps", reason: "fix", changes: SetChanges{Reps: correctionIntPtr(0)}},
	}

	for _, test := range tests {
		_, _, err := NormalizeCorrection(test.reason, test.changes)
		if err == nil {
			t.Fatalf("%s: error = nil, want invalid_input", test.name)
		}
		requireTrainingCode(t, err, "invalid_input")
	}
}

func TestCanonicalSetJSONSortsKeys(t *testing.T) {
	encoded, err := CanonicalSetJSON(correctionFixture())
	if err != nil {
		t.Fatalf("canonical json: %v", err)
	}
	want := `{"created_at":1000,"evidence_source":"reported","exercise_id":"goblet-squat","id":"set-1","idempotency_key":"key-1","kg":20,"note":null,"reps":10,"revision":1,"session_id":"session-1","set_index":1}`
	if encoded != want {
		t.Fatalf("canonical json = %s, want %s", encoded, want)
	}
}

func TestApplyCorrectionNoOpKeepsValuesAndBumpsRevision(t *testing.T) {
	set := correctionFixture()
	before, err := CanonicalSetJSON(set)
	if err != nil {
		t.Fatalf("canonical before: %v", err)
	}

	updated, err := ApplyCorrection(set, SetChanges{KG: correctionFloatPtr(20)})
	if err != nil {
		t.Fatalf("apply correction: %v", err)
	}
	if updated.KG != set.KG || updated.Reps != set.Reps || updated.SetIndex != set.SetIndex || updated.ExerciseID != set.ExerciseID {
		t.Fatalf("no-op correction changed values: %+v", updated)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision = %d, want 2", updated.Revision)
	}

	after, err := CanonicalSetJSON(updated)
	if err != nil {
		t.Fatalf("canonical after: %v", err)
	}
	if after != strings.Replace(before, `"revision":1`, `"revision":2`, 1) {
		t.Fatalf("after = %s, before = %s", after, before)
	}
}

func TestApplyCorrectionReplacesAndClearsNote(t *testing.T) {
	set := correctionFixture()
	set.Note = correctionStringPtr("old")

	updated, err := ApplyCorrection(set, SetChanges{Note: correctionStringPtr("   ")})
	if err != nil {
		t.Fatalf("clear note: %v", err)
	}
	if updated.Note != nil {
		t.Fatalf("note = %q, want nil", *updated.Note)
	}

	updated, err = ApplyCorrection(set, SetChanges{Note: correctionStringPtr(" new ")})
	if err != nil {
		t.Fatalf("replace note: %v", err)
	}
	if updated.Note == nil || *updated.Note != "new" {
		t.Fatalf("note = %v", updated.Note)
	}
}

func TestApplyCorrectionRejectsInvalidMergedValues(t *testing.T) {
	set := correctionFixture()
	_, err := ApplyCorrection(set, SetChanges{SetIndex: correctionIntPtr(0)})
	requireTrainingCode(t, err, "invalid_input")

	_, err = ApplyCorrection(set, SetChanges{KG: correctionFloatPtr(-1)})
	requireTrainingCode(t, err, "invalid_input")

	_, err = ApplyCorrection(set, SetChanges{Reps: correctionIntPtr(0)})
	requireTrainingCode(t, err, "invalid_input")
}

func TestValidateEquipmentLimit(t *testing.T) {
	required := []EquipmentRequirement{{EquipmentID: "adjustable-dumbbell", Quantity: 1}}
	stock := []InventoryStock{{EquipmentID: "adjustable-dumbbell", Quantity: 1, MaxKG: correctionFloatPtr(20)}}

	if err := ValidateEquipmentLimit("goblet-squat", 20, required, stock); err != nil {
		t.Fatalf("limit at the maximum: %v", err)
	}
	err := ValidateEquipmentLimit("goblet-squat", 20.5, required, stock)
	requireTrainingCode(t, err, "equipment_limit")

	if err := ValidateEquipmentLimit("bodyweight", 0, nil, stock); err != nil {
		t.Fatalf("exercise without equipment: %v", err)
	}
}

func TestProgressionDimensions(t *testing.T) {
	targets := []ExerciseTarget{{
		TemplateID: "template-strength",
		TargetSets: correctionIntPtr(3),
		RepsMin:    correctionIntPtr(8),
		RepsMax:    correctionIntPtr(12),
		Tempo:      correctionStringPtr("3-1-1"),
	}}

	dimensions := ProgressionDimensions(false, targets)
	if len(dimensions) != 3 {
		t.Fatalf("dimensions = %+v, want sets, reps and tempo", dimensions)
	}
	if dimensions[0].Name != "sets" || dimensions[0].Dimension != 3 || dimensions[0].Basis != ProgressionBasisPlan {
		t.Fatalf("sets dimension = %+v", dimensions[0])
	}
	if dimensions[1].Name != "reps" || dimensions[1].Dimension != "8-12" {
		t.Fatalf("reps dimension = %+v", dimensions[1])
	}
	if dimensions[2].Name != "tempo" || dimensions[2].Dimension != "3-1-1" {
		t.Fatalf("tempo dimension = %+v", dimensions[2])
	}

	withUnilateral := ProgressionDimensions(true, targets)
	if len(withUnilateral) != 4 || withUnilateral[3].Name != "unilateral" || withUnilateral[3].Dimension != true || withUnilateral[3].Basis != ProgressionBasisCatalogue {
		t.Fatalf("unilateral dimension = %+v", withUnilateral)
	}

	if empty := ProgressionDimensions(false, nil); len(empty) != 0 || empty == nil {
		t.Fatalf("empty dimensions = %+v", empty)
	}
}
