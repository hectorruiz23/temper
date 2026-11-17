package domain

import (
	"math"
	"strings"
)

func DeriveWorkoutPace(workouts []Workout) {
	for index := range workouts {
		deriveWorkoutPace(&workouts[index])
	}
}

func deriveWorkoutPace(workout *Workout) {
	if workout.PaceSecPerKM == nil {
		workout.PaceSecPerKM = paceFor(workout.DurationMS, workout.DistanceM)
	}
	for index := range workout.Splits {
		split := &workout.Splits[index]
		if split.PaceSecPerKM != nil {
			continue
		}
		var duration int64
		if split.DurationMS != nil {
			duration = *split.DurationMS
		}
		split.PaceSecPerKM = paceFor(duration, split.DistanceM)
	}
}

func paceFor(durationMS int64, distanceM *float64) *float64 {
	if durationMS <= 0 || distanceM == nil || *distanceM <= 0 {
		return nil
	}
	pace := float64(durationMS) / 1000 / (*distanceM / 1000)
	rounded := math.Round(pace*10) / 10
	return &rounded
}

func validateWorkoutMetrics(workout Workout) error {
	if workout.AvgHRBPM != nil && *workout.AvgHRBPM <= 0 {
		return invalid("workout avg_hr_bpm must be positive")
	}
	if workout.MaxHRBPM != nil && *workout.MaxHRBPM <= 0 {
		return invalid("workout max_hr_bpm must be positive")
	}
	if workout.AvgHRBPM != nil && workout.MaxHRBPM != nil && *workout.AvgHRBPM > *workout.MaxHRBPM {
		return invalid("workout avg_hr_bpm must not be above max_hr_bpm")
	}
	if workout.CaloriesKcal != nil && *workout.CaloriesKcal < 0 {
		return invalid("workout calories_kcal must not be negative")
	}
	if workout.CadenceSPM != nil && *workout.CadenceSPM < 0 {
		return invalid("workout cadence_spm must not be negative")
	}
	if workout.ElevationGainM != nil && *workout.ElevationGainM < 0 {
		return invalid("workout elevation_gain_m must not be negative")
	}
	if workout.PaceSecPerKM != nil && *workout.PaceSecPerKM <= 0 {
		return invalid("workout pace_sec_per_km must be positive")
	}
	seen := make(map[int]bool, len(workout.Splits))
	for _, split := range workout.Splits {
		if seen[split.Position] {
			return invalid("workout split position %d appears more than once", split.Position)
		}
		seen[split.Position] = true
		if split.Position < 0 {
			return invalid("workout split position must not be negative")
		}
		if strings.TrimSpace(derefString(split.Kind)) != "" && split.DistanceM == nil && split.DurationMS == nil {
			return invalid("workout split %d needs a distance_m or a duration_ms", split.Position)
		}
		if split.DistanceM != nil && *split.DistanceM < 0 {
			return invalid("workout split distance_m must not be negative")
		}
		if split.DurationMS != nil && *split.DurationMS < 0 {
			return invalid("workout split duration_ms must not be negative")
		}
		if split.AvgHRBPM != nil && *split.AvgHRBPM <= 0 {
			return invalid("workout split avg_hr_bpm must be positive")
		}
		if split.PaceSecPerKM != nil && *split.PaceSecPerKM <= 0 {
			return invalid("workout split pace_sec_per_km must be positive")
		}
	}
	return nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
