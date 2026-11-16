package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func trainingInt64Ptr(value int64) *int64 {
	return &value
}

func trainingStringPtr(value string) *string {
	return &value
}

func requireTrainingCode(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want code %q", want)
	}
	var rule kernel.RuleError
	if !errors.As(err, &rule) {
		t.Fatalf("error %v is not a kernel.RuleError", err)
	}
	if rule.Code != want {
		t.Fatalf("code = %q, want %q (message %q)", rule.Code, want, rule.Message)
	}
}

func TestValidateCheckIn(t *testing.T) {
	tests := []struct {
		name  string
		input CheckInInput
		code  string
		want  Evidence
	}{
		{
			name:  "neither source",
			input: CheckInInput{ExpectedRevision: 1},
			code:  "invalid_input",
		},
		{
			name:  "both sources",
			input: CheckInInput{ExpectedRevision: 1, ReportedCompletedAt: trainingInt64Ptr(1000), WatchWorkoutID: trainingStringPtr("watch-1")},
			code:  "invalid_input",
		},
		{
			name:  "negative revision",
			input: CheckInInput{ExpectedRevision: -1, ReportedCompletedAt: trainingInt64Ptr(1000)},
			code:  "invalid_input",
		},
		{
			name:  "empty watch id",
			input: CheckInInput{ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr("  ")},
			code:  "invalid_input",
		},
		{
			name:  "zero reported time",
			input: CheckInInput{ExpectedRevision: 1, ReportedCompletedAt: trainingInt64Ptr(0)},
			code:  "invalid_input",
		},
		{
			name:  "watch",
			input: CheckInInput{ExpectedRevision: 1, WatchWorkoutID: trainingStringPtr(" watch-1 ")},
			want:  Evidence{Source: EvidenceWatch, WatchWorkoutID: trainingStringPtr("watch-1")},
		},
		{
			name:  "reported",
			input: CheckInInput{ExpectedRevision: 1, ReportedCompletedAt: trainingInt64Ptr(1700000000000)},
			want:  Evidence{Source: EvidenceReported, ReportedCompletedAt: trainingInt64Ptr(1700000000000)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence, err := ValidateCheckIn(test.input)
			if test.code != "" {
				requireTrainingCode(t, err, test.code)
				return
			}
			if err != nil {
				t.Fatalf("ValidateCheckIn: %v", err)
			}
			if !reflect.DeepEqual(evidence, test.want) {
				t.Fatalf("evidence = %+v, want %+v", evidence, test.want)
			}
		})
	}
}

func TestUsableWatchWorkout(t *testing.T) {
	tests := []struct {
		name    string
		workout *Workout
		code    string
	}{
		{name: "missing", workout: nil, code: "unusable_watch_evidence"},
		{name: "not present", workout: &Workout{ID: "w", Present: false, WatchVerified: true}, code: "unusable_watch_evidence"},
		{name: "not verified", workout: &Workout{ID: "w", Present: true, WatchVerified: false}, code: "unusable_watch_evidence"},
		{name: "usable", workout: &Workout{ID: "w", Present: true, WatchVerified: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := UsableWatchWorkout(test.workout)
			if test.code != "" {
				requireTrainingCode(t, err, test.code)
				return
			}
			if err != nil {
				t.Fatalf("UsableWatchWorkout: %v", err)
			}
		})
	}
}

func TestIsComplete(t *testing.T) {
	tests := []struct {
		name       string
		watchID    *string
		present    bool
		reportedAt *int64
		want       bool
	}{
		{name: "nothing", want: false},
		{name: "present watch", watchID: trainingStringPtr("w"), present: true, want: true},
		{name: "absent watch", watchID: trainingStringPtr("w"), present: false, want: false},
		{name: "reported", reportedAt: trainingInt64Ptr(1000), want: true},
		{name: "absent watch with reported", watchID: trainingStringPtr("w"), present: false, reportedAt: trainingInt64Ptr(1000), want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsComplete(test.watchID, test.present, test.reportedAt); got != test.want {
				t.Fatalf("IsComplete = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEffectiveEvidence(t *testing.T) {
	session := Session{WatchWorkoutID: trainingStringPtr("w"), ReportedCompletedAt: trainingInt64Ptr(1000)}

	if evidence := EffectiveEvidence(session, true); evidence == nil || evidence.Source != EvidenceWatch || evidence.WatchWorkoutID == nil {
		t.Fatalf("present watch evidence = %+v", evidence)
	}
	if evidence := EffectiveEvidence(session, false); evidence == nil || evidence.Source != EvidenceReported {
		t.Fatalf("reported evidence = %+v", evidence)
	}
	if evidence := EffectiveEvidence(Session{}, false); evidence != nil {
		t.Fatalf("empty evidence = %+v, want nil", evidence)
	}
}

func TestNormalizeSets(t *testing.T) {
	note := "  felt good  "
	sessionID, sets, err := NormalizeSets(" session-1 ", []NewSet{
		{ExerciseID: " goblet-squat ", SetIndex: 1, KG: 20.005, Reps: 10, Note: &note},
		{ExerciseID: "goblet-squat", SetIndex: 2, KG: 20, Reps: 8, Note: trainingStringPtr("   ")},
	})
	if err != nil {
		t.Fatalf("NormalizeSets: %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("session id = %q", sessionID)
	}
	if sets[0].ExerciseID != "goblet-squat" || sets[0].KG != 20.01 || sets[0].Reps != 10 {
		t.Fatalf("first set = %+v", sets[0])
	}
	if sets[0].Note == nil || *sets[0].Note != "felt good" {
		t.Fatalf("first note = %v", sets[0].Note)
	}
	if sets[1].Note != nil {
		t.Fatalf("blank note = %v, want nil", *sets[1].Note)
	}

	tests := []struct {
		name      string
		sessionID string
		sets      []NewSet
	}{
		{name: "missing session", sessionID: " ", sets: []NewSet{{ExerciseID: "x", SetIndex: 1, Reps: 1}}},
		{name: "empty batch", sessionID: "s", sets: nil},
		{name: "missing exercise", sessionID: "s", sets: []NewSet{{ExerciseID: " ", SetIndex: 1, Reps: 1}}},
		{name: "zero position", sessionID: "s", sets: []NewSet{{ExerciseID: "x", SetIndex: 0, Reps: 1}}},
		{name: "negative kg", sessionID: "s", sets: []NewSet{{ExerciseID: "x", SetIndex: 1, KG: -1, Reps: 1}}},
		{name: "zero reps", sessionID: "s", sets: []NewSet{{ExerciseID: "x", SetIndex: 1, Reps: 0}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := NormalizeSets(test.sessionID, test.sets)
			requireTrainingCode(t, err, "invalid_input")
		})
	}
}

func TestValidateNewSession(t *testing.T) {
	template := " template-1 "
	normalized, err := ValidateNewSession(NewSession{
		ID:         " session-1 ",
		TemplateID: &template,
		Date:       "2026-12-02",
		Discipline: DisciplineStrength,
	})
	if err != nil {
		t.Fatalf("ValidateNewSession: %v", err)
	}
	if normalized.ID != "session-1" || normalized.TemplateID == nil || *normalized.TemplateID != "template-1" {
		t.Fatalf("normalized = %+v", normalized)
	}

	blank := " "
	blanked, err := ValidateNewSession(NewSession{ID: "s", TemplateID: &blank, Date: "2026-12-02", Discipline: DisciplineRun})
	if err != nil {
		t.Fatalf("blank template: %v", err)
	}
	if blanked.TemplateID != nil {
		t.Fatalf("blank template = %v, want nil", *blanked.TemplateID)
	}

	tests := []struct {
		name  string
		input NewSession
	}{
		{name: "missing id", input: NewSession{Date: "2026-12-02", Discipline: DisciplineRun}},
		{name: "bad date", input: NewSession{ID: "s", Date: "2026-02-30", Discipline: DisciplineRun}},
		{name: "bad discipline", input: NewSession{ID: "s", Date: "2026-12-02", Discipline: "swim"}},
		{name: "zero started at", input: NewSession{ID: "s", Date: "2026-12-02", Discipline: DisciplineRun, StartedAt: trainingInt64Ptr(0)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateNewSession(test.input)
			requireTrainingCode(t, err, "invalid_input")
		})
	}
}
