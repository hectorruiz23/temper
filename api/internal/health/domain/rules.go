package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func ValidateDate(value string) error {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return invalid("invalid date")
	}
	return nil
}

func NextRevision(existing *Day, expected int64) (int64, error) {
	if expected < 0 {
		return 0, invalid("expected_revision must not be negative")
	}
	if existing == nil {
		if expected != 0 {
			return 0, conflict("expected revision %d does not match a missing day", expected)
		}
		return 1, nil
	}
	if expected != existing.Revision {
		return 0, conflict("expected revision %d does not match stored revision %d", expected, existing.Revision)
	}
	return existing.Revision + 1, nil
}

func ValidateUpsert(upsert Upsert) error {
	if err := ValidateDate(upsert.Date); err != nil {
		return err
	}
	if strings.TrimSpace(upsert.Timezone) == "" {
		return invalid("timezone is required")
	}
	if upsert.Source != SourceManual && upsert.Source != SourceHealthConnect {
		return invalid("source must be manual or health_connect")
	}
	if upsert.ExpectedRevision < 0 {
		return invalid("expected_revision must not be negative")
	}
	if err := validateMetrics(upsert.Metrics); err != nil {
		return err
	}
	if upsert.SleepStages != nil {
		if err := validateStages(*upsert.SleepStages); err != nil {
			return err
		}
	}
	if upsert.Workouts != nil {
		if err := validateWorkouts(*upsert.Workouts); err != nil {
			return err
		}
	}
	return nil
}

func ApplyMetrics(day Day, patch MetricsPatch) Day {
	if patch.SleepMS.Set {
		day.SleepMS = cloneInt64(patch.SleepMS.Value)
	}
	if patch.Steps.Set {
		day.Steps = cloneInt64(patch.Steps.Value)
	}
	if patch.RestingHRBPM.Set {
		day.RestingHRBPM = cloneInt64(patch.RestingHRBPM.Value)
	}
	if patch.HRVRMSSDMS.Set {
		day.HRVRMSSDMS = cloneFloat64(patch.HRVRMSSDMS.Value)
	}
	return day
}

func validateMetrics(metrics MetricsPatch) error {
	if metrics.SleepMS.Set && metrics.SleepMS.Value != nil && *metrics.SleepMS.Value < 0 {
		return invalid("sleep_ms must not be negative")
	}
	if metrics.Steps.Set && metrics.Steps.Value != nil && *metrics.Steps.Value < 0 {
		return invalid("steps must not be negative")
	}
	if metrics.RestingHRBPM.Set && metrics.RestingHRBPM.Value != nil && *metrics.RestingHRBPM.Value <= 0 {
		return invalid("resting_hr_bpm must be positive")
	}
	if metrics.HRVRMSSDMS.Set && metrics.HRVRMSSDMS.Value != nil && *metrics.HRVRMSSDMS.Value < 0 {
		return invalid("hrv_rmssd_ms must not be negative")
	}
	return nil
}

func validateStages(stages []SleepStage) error {
	seen := make(map[string]bool, len(stages))
	for _, stage := range stages {
		if strings.TrimSpace(stage.Stage) == "" {
			return invalid("sleep stage name is required")
		}
		if seen[stage.Stage] {
			return invalid("sleep stage %s appears more than once", stage.Stage)
		}
		seen[stage.Stage] = true
		if stage.DurationMS < 0 {
			return invalid("sleep stage duration_ms must not be negative")
		}
	}
	return nil
}

func validateWorkouts(workouts []Workout) error {
	seen := make(map[string]bool, len(workouts))
	for _, workout := range workouts {
		if strings.TrimSpace(workout.Origin) == "" {
			return invalid("workout origin is required")
		}
		if strings.TrimSpace(workout.ExternalID) == "" {
			return invalid("workout external_id is required")
		}
		if strings.TrimSpace(workout.Discipline) == "" {
			return invalid("workout discipline is required")
		}
		if workout.EndedAt < workout.StartedAt {
			return invalid("workout ended_at must not be before started_at")
		}
		if workout.DurationMS < 0 {
			return invalid("workout duration_ms must not be negative")
		}
		if workout.DistanceM != nil && *workout.DistanceM < 0 {
			return invalid("workout distance_m must not be negative")
		}
		key := workout.Origin + "\x00" + workout.ExternalID
		if seen[key] {
			return invalid("workout %s/%s appears more than once", workout.Origin, workout.ExternalID)
		}
		seen[key] = true
	}
	return nil
}

func invalid(format string, arguments ...any) error {
	return kernel.NewRuleError("invalid_input", fmt.Sprintf(format, arguments...))
}

func conflict(format string, arguments ...any) error {
	return kernel.NewRuleError("revision_conflict", fmt.Sprintf(format, arguments...))
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
