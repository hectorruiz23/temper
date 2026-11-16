package domain

import (
	"errors"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func intPtr(value int64) *int64 {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func ruleCode(t *testing.T, err error) string {
	t.Helper()

	var rule kernel.RuleError
	if !errors.As(err, &rule) {
		t.Fatalf("error %v is not a kernel.RuleError", err)
	}
	return rule.Code
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name  string
		date  string
		valid bool
	}{
		{name: "valid", date: "2026-11-16", valid: true},
		{name: "leap day", date: "2028-02-29", valid: true},
		{name: "impossible day", date: "2026-02-30", valid: false},
		{name: "month out of range", date: "2026-13-01", valid: false},
		{name: "day out of range", date: "2026-04-31", valid: false},
		{name: "single digit", date: "2026-1-1", valid: false},
		{name: "slashes", date: "16/11/2026", valid: false},
		{name: "empty", date: "", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDate(test.date)
			if test.valid && err != nil {
				t.Fatalf("ValidateDate(%q) = %v, want nil", test.date, err)
			}
			if !test.valid {
				if err == nil {
					t.Fatalf("ValidateDate(%q) = nil, want invalid_input", test.date)
				}
				if code := ruleCode(t, err); code != "invalid_input" {
					t.Fatalf("code = %q, want invalid_input", code)
				}
			}
		})
	}
}

func TestNextRevision(t *testing.T) {
	tests := []struct {
		name     string
		existing *Day
		expected int64
		want     int64
		code     string
	}{
		{name: "create at zero", existing: nil, expected: 0, want: 1},
		{name: "create at non-zero", existing: nil, expected: 3, code: "revision_conflict"},
		{name: "update current", existing: &Day{Revision: 3}, expected: 3, want: 4},
		{name: "update stale", existing: &Day{Revision: 3}, expected: 2, code: "revision_conflict"},
		{name: "update treating as create", existing: &Day{Revision: 3}, expected: 0, code: "revision_conflict"},
		{name: "negative", existing: nil, expected: -1, code: "invalid_input"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextRevision(test.existing, test.expected)
			if test.code == "" {
				if err != nil {
					t.Fatalf("NextRevision = %v, want %d", err, test.want)
				}
				if got != test.want {
					t.Fatalf("revision = %d, want %d", got, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("NextRevision = %d, want %s", got, test.code)
			}
			if code := ruleCode(t, err); code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}

func validUpsert() Upsert {
	return Upsert{
		Date:     "2026-11-16",
		Timezone: "Europe/Madrid",
		Source:   SourceManual,
	}
}

func TestValidateUpsertMetrics(t *testing.T) {
	tests := []struct {
		name    string
		metrics MetricsPatch
		valid   bool
	}{
		{name: "omitted", metrics: MetricsPatch{}, valid: true},
		{name: "null", metrics: MetricsPatch{SleepMS: OptionalInt64{Set: true}}, valid: true},
		{name: "zero", metrics: MetricsPatch{SleepMS: OptionalInt64{Set: true, Value: intPtr(0)}}, valid: true},
		{name: "negative sleep", metrics: MetricsPatch{SleepMS: OptionalInt64{Set: true, Value: intPtr(-1)}}, valid: false},
		{name: "negative steps", metrics: MetricsPatch{Steps: OptionalInt64{Set: true, Value: intPtr(-1)}}, valid: false},
		{name: "zero resting heart rate", metrics: MetricsPatch{RestingHRBPM: OptionalInt64{Set: true, Value: intPtr(0)}}, valid: false},
		{name: "negative heart rate", metrics: MetricsPatch{RestingHRBPM: OptionalInt64{Set: true, Value: intPtr(-2)}}, valid: false},
		{name: "negative hrv", metrics: MetricsPatch{HRVRMSSDMS: OptionalFloat64{Set: true, Value: floatPtr(-0.1)}}, valid: false},
		{name: "zero hrv", metrics: MetricsPatch{HRVRMSSDMS: OptionalFloat64{Set: true, Value: floatPtr(0)}}, valid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upsert := validUpsert()
			upsert.Metrics = test.metrics
			err := ValidateUpsert(upsert)
			if test.valid && err != nil {
				t.Fatalf("ValidateUpsert = %v, want nil", err)
			}
			if !test.valid {
				if err == nil {
					t.Fatal("ValidateUpsert = nil, want invalid_input")
				}
				if code := ruleCode(t, err); code != "invalid_input" {
					t.Fatalf("code = %q, want invalid_input", code)
				}
			}
		})
	}
}

func TestValidateUpsertStages(t *testing.T) {
	tests := []struct {
		name   string
		stages []SleepStage
		valid  bool
	}{
		{name: "empty replacement", stages: []SleepStage{}, valid: true},
		{name: "accepted", stages: []SleepStage{{Stage: "deep", DurationMS: 7200000}, {Stage: "rem", DurationMS: 0}}, valid: true},
		{name: "missing name", stages: []SleepStage{{Stage: "", DurationMS: 1}}, valid: false},
		{name: "duplicate", stages: []SleepStage{{Stage: "deep", DurationMS: 1}, {Stage: "deep", DurationMS: 2}}, valid: false},
		{name: "negative duration", stages: []SleepStage{{Stage: "deep", DurationMS: -1}}, valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upsert := validUpsert()
			upsert.SleepStages = &test.stages
			err := ValidateUpsert(upsert)
			if test.valid && err != nil {
				t.Fatalf("ValidateUpsert = %v, want nil", err)
			}
			if !test.valid {
				if err == nil {
					t.Fatal("ValidateUpsert = nil, want invalid_input")
				}
				if code := ruleCode(t, err); code != "invalid_input" {
					t.Fatalf("code = %q, want invalid_input", code)
				}
			}
		})
	}
}

func TestValidateUpsertWorkouts(t *testing.T) {
	valid := func() Workout {
		return Workout{
			Origin:        "health_connect",
			ExternalID:    "abc-123",
			DeviceKind:    stringPtr("watch"),
			StartedAt:     1763300000000,
			EndedAt:       1763303600000,
			Discipline:    "run",
			DurationMS:    3600000,
			DistanceM:     floatPtr(5200),
			WatchVerified: true,
		}
	}

	tests := []struct {
		name      string
		mutate    func(*Workout)
		valid     bool
		duplicate bool
	}{
		{name: "accepted", mutate: func(*Workout) {}, valid: true},
		{name: "null distance", mutate: func(workout *Workout) { workout.DistanceM = nil }, valid: true},
		{name: "zero distance", mutate: func(workout *Workout) { workout.DistanceM = floatPtr(0) }, valid: true},
		{name: "missing origin", mutate: func(workout *Workout) { workout.Origin = "" }, valid: false},
		{name: "missing external id", mutate: func(workout *Workout) { workout.ExternalID = "" }, valid: false},
		{name: "missing discipline", mutate: func(workout *Workout) { workout.Discipline = "" }, valid: false},
		{name: "end before start", mutate: func(workout *Workout) { workout.EndedAt = workout.StartedAt - 1 }, valid: false},
		{name: "negative duration", mutate: func(workout *Workout) { workout.DurationMS = -1 }, valid: false},
		{name: "negative distance", mutate: func(workout *Workout) { workout.DistanceM = floatPtr(-1) }, valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workout := valid()
			test.mutate(&workout)
			upsert := validUpsert()
			upsert.Workouts = &[]Workout{workout}
			err := ValidateUpsert(upsert)
			if test.valid && err != nil {
				t.Fatalf("ValidateUpsert = %v, want nil", err)
			}
			if !test.valid {
				if err == nil {
					t.Fatal("ValidateUpsert = nil, want invalid_input")
				}
				if code := ruleCode(t, err); code != "invalid_input" {
					t.Fatalf("code = %q, want invalid_input", code)
				}
			}
		})
	}

	workout := valid()
	upsert := validUpsert()
	upsert.Workouts = &[]Workout{workout, workout}
	if err := ValidateUpsert(upsert); err == nil {
		t.Fatal("duplicate workouts accepted")
	} else if code := ruleCode(t, err); code != "invalid_input" {
		t.Fatalf("code = %q, want invalid_input", code)
	}
}

func TestValidateUpsertTopLevel(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Upsert)
		valid  bool
	}{
		{name: "accepted", mutate: func(*Upsert) {}, valid: true},
		{name: "health connect", mutate: func(upsert *Upsert) { upsert.Source = SourceHealthConnect }, valid: true},
		{name: "bad date", mutate: func(upsert *Upsert) { upsert.Date = "2026-02-30" }, valid: false},
		{name: "missing timezone", mutate: func(upsert *Upsert) { upsert.Timezone = "" }, valid: false},
		{name: "bad source", mutate: func(upsert *Upsert) { upsert.Source = "watch" }, valid: false},
		{name: "negative expected revision", mutate: func(upsert *Upsert) { upsert.ExpectedRevision = -1 }, valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upsert := validUpsert()
			test.mutate(&upsert)
			err := ValidateUpsert(upsert)
			if test.valid && err != nil {
				t.Fatalf("ValidateUpsert = %v, want nil", err)
			}
			if !test.valid {
				if err == nil {
					t.Fatal("ValidateUpsert = nil, want invalid_input")
				}
				if code := ruleCode(t, err); code != "invalid_input" {
					t.Fatalf("code = %q, want invalid_input", code)
				}
			}
		})
	}
}

func TestApplyMetricsDistinguishesOmittedFromNull(t *testing.T) {
	day := Day{
		SleepMS:      intPtr(100),
		Steps:        intPtr(10),
		RestingHRBPM: intPtr(50),
		HRVRMSSDMS:   floatPtr(40),
	}

	omitted := ApplyMetrics(day, MetricsPatch{})
	if omitted.SleepMS == nil || *omitted.SleepMS != 100 {
		t.Fatalf("omitted metric changed: %v", omitted.SleepMS)
	}

	null := ApplyMetrics(day, MetricsPatch{
		SleepMS:    OptionalInt64{Set: true},
		HRVRMSSDMS: OptionalFloat64{Set: true},
	})
	if null.SleepMS != nil {
		t.Fatalf("null sleep_ms = %v, want nil", *null.SleepMS)
	}
	if null.HRVRMSSDMS != nil {
		t.Fatalf("null hrv_rmssd_ms = %v, want nil", *null.HRVRMSSDMS)
	}
	if null.Steps == nil || *null.Steps != 10 {
		t.Fatalf("unset steps changed: %v", null.Steps)
	}

	value := ApplyMetrics(day, MetricsPatch{
		Steps:        OptionalInt64{Set: true, Value: intPtr(0)},
		RestingHRBPM: OptionalInt64{Set: true, Value: intPtr(0)},
	})
	if value.Steps == nil || *value.Steps != 0 {
		t.Fatalf("explicit zero steps = %v, want 0", value.Steps)
	}
	if value.RestingHRBPM == nil || *value.RestingHRBPM != 0 {
		t.Fatalf("explicit zero heart rate = %v, want 0", value.RestingHRBPM)
	}
}

func TestReconcileWorkouts(t *testing.T) {
	existing := []Workout{
		{ID: "id-a", Origin: "health_connect", ExternalID: "a", StartedAt: 1, EndedAt: 2, Discipline: "run", Present: true},
		{ID: "id-b", Origin: "health_connect", ExternalID: "b", StartedAt: 3, EndedAt: 4, Discipline: "run", Present: true},
	}
	incoming := []Workout{
		{Origin: "health_connect", ExternalID: "a", StartedAt: 10, EndedAt: 20, Discipline: "ride"},
		{Origin: "health_connect", ExternalID: "c", StartedAt: 30, EndedAt: 40, Discipline: "swim"},
	}

	result, err := ReconcileWorkouts(existing, incoming)
	if err != nil {
		t.Fatalf("ReconcileWorkouts: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	byExternalID := make(map[string]Workout, len(result))
	for _, workout := range result {
		byExternalID[workout.ExternalID] = workout
	}
	if byExternalID["a"].ID != "id-a" || !byExternalID["a"].Present || byExternalID["a"].StartedAt != 10 {
		t.Fatalf("reappearing workout = %+v", byExternalID["a"])
	}
	if byExternalID["b"].ID != "id-b" || byExternalID["b"].Present {
		t.Fatalf("disappearing workout = %+v", byExternalID["b"])
	}
	if byExternalID["c"].ID == "" || !byExternalID["c"].Present {
		t.Fatalf("new workout = %+v", byExternalID["c"])
	}

	again, err := ReconcileWorkouts(result, []Workout{{Origin: "health_connect", ExternalID: "b", StartedAt: 3, EndedAt: 4, Discipline: "run"}})
	if err != nil {
		t.Fatalf("second ReconcileWorkouts: %v", err)
	}
	for _, workout := range again {
		if workout.ExternalID == "b" {
			if workout.ID != "id-b" || !workout.Present {
				t.Fatalf("reappearing workout did not keep its id: %+v", workout)
			}
		}
	}
}
