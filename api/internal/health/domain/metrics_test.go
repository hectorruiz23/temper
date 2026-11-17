package domain

import "testing"

func testWorkout(id string) Workout {
	return Workout{ID: id, Origin: "manual", ExternalID: id, Discipline: "run"}
}

func TestPaceIsDerivedFromDurationAndDistance(t *testing.T) {
	workout := testWorkout("watch-run-1")
	workout.Origin = "health_connect"
	workout.DurationMS = 1740000
	workout.DistanceM = floatPtr(5230)
	workout.WatchVerified = true
	workout.Splits = []WorkoutSplit{
		{Position: 0, DistanceM: floatPtr(1000), DurationMS: intPtr(330000)},
		{Position: 1, DistanceM: floatPtr(1000), DurationMS: intPtr(336000), PaceSecPerKM: floatPtr(300)},
	}

	DeriveWorkoutPace([]Workout{workout})

	reconciled, err := ReconcileWorkouts(nil, []Workout{workout})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	derived := reconciled[0]
	if derived.PaceSecPerKM == nil || !closeEnough(*derived.PaceSecPerKM, 332.7) {
		t.Fatalf("pace = %v, want 332.7", derived.PaceSecPerKM)
	}
	if derived.Splits[0].PaceSecPerKM == nil || !closeEnough(*derived.Splits[0].PaceSecPerKM, 330) {
		t.Fatalf("split pace = %v, want 330", derived.Splits[0].PaceSecPerKM)
	}
	if derived.Splits[1].PaceSecPerKM == nil || *derived.Splits[1].PaceSecPerKM != 300 {
		t.Fatalf("explicit split pace = %v, want 300", derived.Splits[1].PaceSecPerKM)
	}
}

func TestPaceIsAbsentWithoutDistance(t *testing.T) {
	workout := testWorkout("note-1")
	workout.DurationMS = 1800000

	reconciled, err := ReconcileWorkouts(nil, []Workout{workout})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled[0].PaceSecPerKM != nil {
		t.Fatalf("pace = %v, want absent", *reconciled[0].PaceSecPerKM)
	}
}

func TestWorkoutRejectsImpossibleMetrics(t *testing.T) {
	tests := []struct {
		name    string
		workout Workout
	}{
		{
			name: "average heart rate above the maximum",
			workout: func() Workout {
				workout := testWorkout("a")
				workout.AvgHRBPM = intPtr(150)
				workout.MaxHRBPM = intPtr(120)
				return workout
			}(),
		},
		{
			name: "zero heart rate",
			workout: func() Workout {
				workout := testWorkout("b")
				workout.AvgHRBPM = intPtr(0)
				return workout
			}(),
		},
		{
			name: "negative calories",
			workout: func() Workout {
				workout := testWorkout("c")
				workout.CaloriesKcal = floatPtr(-1)
				return workout
			}(),
		},
		{
			name: "negative elevation",
			workout: func() Workout {
				workout := testWorkout("d")
				workout.ElevationGainM = floatPtr(-5)
				return workout
			}(),
		},
		{
			name: "zero pace",
			workout: func() Workout {
				workout := testWorkout("e")
				workout.PaceSecPerKM = floatPtr(0)
				return workout
			}(),
		},
		{
			name: "duplicated split position",
			workout: func() Workout {
				workout := testWorkout("f")
				workout.Splits = []WorkoutSplit{
					{Position: 0, DistanceM: floatPtr(1000)},
					{Position: 0, DistanceM: floatPtr(1000)},
				}
				return workout
			}(),
		},
		{
			name: "split without distance or duration",
			workout: func() Workout {
				workout := testWorkout("g")
				workout.Splits = []WorkoutSplit{{Position: 0, Kind: stringPtr("km")}}
				return workout
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReconcileWorkouts(nil, []Workout{test.workout})
			if err == nil {
				t.Fatalf("workout %+v was accepted", test.workout)
			}
			if code := ruleCode(t, err); code != "invalid_input" {
				t.Fatalf("code = %q, want invalid_input", code)
			}
		})
	}
}

func TestWorkoutKeepsNullMetrics(t *testing.T) {
	workout := testWorkout("note-1")
	workout.DurationMS = 1800000
	workout.DistanceM = floatPtr(5000)

	reconciled, err := ReconcileWorkouts(nil, []Workout{workout})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	stored := reconciled[0]
	if stored.AvgHRBPM != nil || stored.MaxHRBPM != nil || stored.CaloriesKcal != nil || stored.CadenceSPM != nil || stored.ElevationGainM != nil {
		t.Fatalf("absent metrics were filled in: %+v", stored)
	}
	if stored.PaceSecPerKM == nil {
		t.Fatalf("pace should be derived from duration and distance")
	}
}

func closeEnough(got, want float64) bool {
	difference := got - want
	return difference < 0.05 && difference > -0.05
}
