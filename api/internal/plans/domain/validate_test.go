package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func validPlan() Plan {
	return Plan{
		Reason:          "base strength block",
		EffectiveFrom:   "2026-12-01",
		Timezone:        "Europe/Madrid",
		WeeklyFrequency: 3,
		GoalID:          "race-5k-2026-12-20",
		Sessions: []Session{
			{
				ID:            "session-a",
				Position:      0,
				ScheduledDate: stringPtr("2026-12-02"),
				Discipline:    DisciplineStrength,
				Target:        "full body",
				Exercises: []SessionExercise{
					{
						Position:   0,
						ExerciseID: "goblet-squat",
						TargetSets: intPtr(3),
						RepsMin:    intPtr(8),
						RepsMax:    intPtr(12),
						TargetKG:   floatPtr(20),
						Tempo:      stringPtr("3-1-1"),
					},
				},
			},
		},
	}
}

func validIntervalsPlan() Plan {
	plan := validPlan()
	plan.Sessions = []Session{
		{
			ID:            "session-b",
			Position:      0,
			ScheduledDate: stringPtr("2026-12-03"),
			Discipline:    DisciplineIntervals,
			Steps: []IntervalStep{
				{Position: 0, Label: "warm up", DurationMS: intPtr(300000)},
				{Position: 1, Label: "repetition", DistanceM: floatPtr(400)},
			},
		},
	}
	return plan
}

func requireCode(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want code %q", want)
	}
	var ruleError kernel.RuleError
	if !errors.As(err, &ruleError) {
		t.Fatalf("error %v is not a kernel.RuleError", err)
	}
	if ruleError.Code != want {
		t.Fatalf("code = %q, want %q (message %q)", ruleError.Code, want, ruleError.Message)
	}
	if ruleError.Message == "" {
		t.Fatalf("code %q has an empty message", want)
	}
}

func TestValidatePlan(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		mutate func(*Plan)
	}{
		{
			name: "empty reason",
			code: "invalid_input",
			mutate: func(plan *Plan) {
				plan.Reason = "  "
			},
		},
		{
			name: "zero weekly frequency",
			code: "invalid_input",
			mutate: func(plan *Plan) {
				plan.WeeklyFrequency = 0
			},
		},
		{
			name: "unknown discipline",
			code: "invalid_input",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = "swim"
			},
		},
		{
			name: "empty session id",
			code: "invalid_input",
			mutate: func(plan *Plan) {
				plan.Sessions[0].ID = " "
			},
		},
		{
			name: "empty exercise id",
			code: "invalid_input",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Exercises[0].ExerciseID = ""
			},
		},
		{
			name: "effective_from is not a date",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.EffectiveFrom = "2026-13-40"
			},
		},
		{
			name: "empty timezone",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Timezone = ""
			},
		},
		{
			name: "unknown timezone",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Timezone = "Mars/Olympus"
			},
		},
		{
			name: "neither date nor weekday",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].ScheduledDate = nil
			},
		},
		{
			name: "both date and weekday",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Weekday = intPtr(3)
			},
		},
		{
			name: "weekday below one",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].ScheduledDate = nil
				plan.Sessions[0].Weekday = intPtr(0)
			},
		},
		{
			name: "weekday above seven",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].ScheduledDate = nil
				plan.Sessions[0].Weekday = intPtr(8)
			},
		},
		{
			name: "scheduled date is not a date",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].ScheduledDate = stringPtr("tomorrow")
			},
		},
		{
			name: "negative session position",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Position = -1
			},
		},
		{
			name: "duplicated session position",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				duplicate := plan.Sessions[0]
				duplicate.ID = "session-b"
				plan.Sessions = append(plan.Sessions, duplicate)
			},
		},
		{
			name: "duplicated exercise position",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				duplicate := plan.Sessions[0].Exercises[0]
				duplicate.ExerciseID = "other"
				plan.Sessions[0].Exercises = append(plan.Sessions[0].Exercises, duplicate)
			},
		},
		{
			name: "target sets zero",
			code: "invalid_target",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Exercises[0].TargetSets = intPtr(0)
			},
		},
		{
			name: "negative reps_min",
			code: "invalid_target",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Exercises[0].RepsMin = intPtr(-1)
			},
		},
		{
			name: "reps_min greater than reps_max",
			code: "invalid_target",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Exercises[0].RepsMin = intPtr(12)
				plan.Sessions[0].Exercises[0].RepsMax = intPtr(8)
			},
		},
		{
			name: "negative target_kg",
			code: "invalid_target",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Exercises[0].TargetKG = floatPtr(-1)
			},
		},
		{
			name: "intervals without steps",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
			},
		},
		{
			name: "steps on a non intervals session",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: "extra", DurationMS: intPtr(1000)}}
			},
		},
		{
			name: "step with both measurements",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: "both", DurationMS: intPtr(1000), DistanceM: floatPtr(400)}}
			},
		},
		{
			name: "step without measurement",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: "none"}}
			},
		},
		{
			name: "step with zero duration",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: "zero", DurationMS: intPtr(0)}}
			},
		},
		{
			name: "step with zero distance",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: "zero", DistanceM: floatPtr(0)}}
			},
		},
		{
			name: "step without label",
			code: "invalid_interval_steps",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{{Position: 0, Label: " ", DurationMS: intPtr(1000)}}
			},
		},
		{
			name: "duplicated step position",
			code: "invalid_schedule",
			mutate: func(plan *Plan) {
				plan.Sessions[0].Discipline = DisciplineIntervals
				plan.Sessions[0].Exercises = nil
				plan.Sessions[0].Steps = []IntervalStep{
					{Position: 0, Label: "first", DurationMS: intPtr(1000)},
					{Position: 0, Label: "second", DurationMS: intPtr(2000)},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			test.mutate(&plan)
			_, err := ValidatePlan(plan)
			requireCode(t, err, test.code)
		})
	}
}

func TestValidatePlanAcceptsStrengthAndIntervals(t *testing.T) {
	normalized, err := ValidatePlan(validPlan())
	if err != nil {
		t.Fatalf("validate strength plan: %v", err)
	}
	if len(normalized.Sessions) != 1 || len(normalized.Sessions[0].Exercises) != 1 {
		t.Fatalf("normalized plan lost children: %+v", normalized)
	}

	if _, err := ValidatePlan(validIntervalsPlan()); err != nil {
		t.Fatalf("validate intervals plan: %v", err)
	}
}

func TestValidatePlanNormalizes(t *testing.T) {
	plan := validPlan()
	plan.Reason = "  base block  "
	plan.Timezone = " Europe/Madrid "
	plan.Sessions[0].ID = " session-a "
	plan.Sessions[0].Target = " full body "
	plan.Sessions[0].Exercises[0].Tempo = stringPtr(" 3-1-1 ")

	extra := plan.Sessions[0]
	extra.ID = "session-z"
	extra.Position = 1
	extra.ScheduledDate = stringPtr("2026-12-09")
	plan.Sessions = append([]Session{extra}, plan.Sessions...)

	normalized, err := ValidatePlan(plan)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if normalized.Reason != "base block" {
		t.Fatalf("reason = %q", normalized.Reason)
	}
	if normalized.Timezone != "Europe/Madrid" {
		t.Fatalf("timezone = %q", normalized.Timezone)
	}
	if normalized.Sessions[0].ID != "session-a" {
		t.Fatalf("sessions are not ordered by position: %+v", normalized.Sessions)
	}
	if normalized.Sessions[0].Exercises[0].Tempo == nil || *normalized.Sessions[0].Exercises[0].Tempo != "3-1-1" {
		t.Fatalf("tempo was not trimmed")
	}
}

func TestCheckEquipment(t *testing.T) {
	maxKG := 20.0
	stock := []InventoryStock{{EquipmentID: "adjustable-dumbbell", Quantity: 1, MaxKG: &maxKG}}

	tests := []struct {
		name     string
		required []EquipmentRequirement
		stock    []InventoryStock
		targetKG *float64
		wantCode string
	}{
		{
			name:     "compatible",
			required: []EquipmentRequirement{{EquipmentID: "adjustable-dumbbell", Quantity: 1}},
			stock:    stock,
			targetKG: floatPtr(20),
		},
		{
			name:     "missing equipment",
			required: []EquipmentRequirement{{EquipmentID: "barbell", Quantity: 1}},
			stock:    stock,
			wantCode: "unavailable_equipment",
		},
		{
			name:     "quantity exceeds inventory",
			required: []EquipmentRequirement{{EquipmentID: "adjustable-dumbbell", Quantity: 2}},
			stock:    stock,
			wantCode: "unavailable_equipment",
		},
		{
			name:     "target above the inventory maximum",
			required: []EquipmentRequirement{{EquipmentID: "adjustable-dumbbell", Quantity: 1}},
			stock:    stock,
			targetKG: floatPtr(21),
			wantCode: "equipment_limit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckEquipment("goblet-squat", test.required, test.stock, test.targetKG)
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("CheckEquipment: %v", err)
				}
				return
			}
			requireCode(t, err, test.wantCode)
		})
	}
}

func TestValidateExercise(t *testing.T) {
	valid := Exercise{
		ID:              " goblet-squat ",
		Name:            " Goblet squat ",
		MovementPattern: " squat ",
		Equipment: []ExerciseEquipment{
			{EquipmentID: "bench", Quantity: 1},
			{EquipmentID: "adjustable-dumbbell", Quantity: 1},
		},
	}

	normalized, err := ValidateExercise(valid)
	if err != nil {
		t.Fatalf("validate exercise: %v", err)
	}
	if normalized.ID != "goblet-squat" || normalized.Name != "Goblet squat" || normalized.MovementPattern != "squat" {
		t.Fatalf("exercise was not trimmed: %+v", normalized)
	}
	if normalized.Equipment[0].EquipmentID != "adjustable-dumbbell" || normalized.Equipment[1].EquipmentID != "bench" {
		t.Fatalf("equipment is not sorted: %+v", normalized.Equipment)
	}

	tests := []struct {
		name   string
		mutate func(*Exercise)
		code   string
	}{
		{name: "missing id", mutate: func(e *Exercise) { e.ID = "" }, code: "invalid_input"},
		{name: "missing name", mutate: func(e *Exercise) { e.Name = "" }, code: "invalid_input"},
		{name: "missing movement pattern", mutate: func(e *Exercise) { e.MovementPattern = "" }, code: "invalid_input"},
		{name: "empty equipment id", mutate: func(e *Exercise) { e.Equipment[0].EquipmentID = "" }, code: "invalid_input"},
		{name: "zero quantity", mutate: func(e *Exercise) { e.Equipment[0].Quantity = 0 }, code: "invalid_input"},
		{name: "duplicated equipment", mutate: func(e *Exercise) { e.Equipment[1].EquipmentID = e.Equipment[0].EquipmentID }, code: "invalid_input"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exercise := valid
			exercise.Equipment = append([]ExerciseEquipment(nil), valid.Equipment...)
			test.mutate(&exercise)
			_, err := ValidateExercise(exercise)
			requireCode(t, err, test.code)
		})
	}
}

func TestValidatePlanDoesNotMutateInputSlices(t *testing.T) {
	plan := validPlan()
	original := append([]Session(nil), plan.Sessions...)
	if _, err := ValidatePlan(plan); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !reflect.DeepEqual(plan.Sessions, original) {
		t.Fatalf("input sessions changed")
	}
}
