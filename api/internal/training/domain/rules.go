package domain

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func ValidateNewSession(input NewSession) (NewSession, error) {
	normalized := input
	normalized.ID = strings.TrimSpace(input.ID)
	normalized.Date = strings.TrimSpace(input.Date)
	normalized.Discipline = strings.TrimSpace(input.Discipline)
	if input.TemplateID != nil {
		templateID := strings.TrimSpace(*input.TemplateID)
		if templateID == "" {
			normalized.TemplateID = nil
		} else {
			normalized.TemplateID = &templateID
		}
	}

	if normalized.ID == "" {
		return NewSession{}, invalid("session id is required")
	}
	if !isISODate(normalized.Date) {
		return NewSession{}, invalid("date %q is not an ISO date", normalized.Date)
	}
	switch normalized.Discipline {
	case DisciplineStrength, DisciplineRun, DisciplineIntervals:
	default:
		return NewSession{}, invalid("discipline %q is not strength, run or intervals", normalized.Discipline)
	}
	if normalized.StartedAt != nil && *normalized.StartedAt <= 0 {
		return NewSession{}, invalid("started_at must be a positive timestamp")
	}

	return normalized, nil
}

func ValidateTemplateDiscipline(discipline, templateDiscipline string) error {
	if discipline != templateDiscipline {
		return kernel.NewRuleError("template_mismatch", fmt.Sprintf("session discipline %q does not match template discipline %q", discipline, templateDiscipline))
	}
	return nil
}

func ValidateCheckIn(input CheckInInput) (Evidence, error) {
	if input.ExpectedRevision < 0 {
		return Evidence{}, invalid("expected_revision must not be negative")
	}

	hasReported := input.ReportedCompletedAt != nil
	hasWatch := input.WatchWorkoutID != nil
	if hasReported == hasWatch {
		return Evidence{}, invalid("exactly one of reported_completed_at and watch_workout_id is required")
	}

	if hasWatch {
		watchID := strings.TrimSpace(*input.WatchWorkoutID)
		if watchID == "" {
			return Evidence{}, invalid("watch_workout_id must not be empty")
		}
		return Evidence{Source: EvidenceWatch, WatchWorkoutID: &watchID}, nil
	}

	if *input.ReportedCompletedAt <= 0 {
		return Evidence{}, invalid("reported_completed_at must be a positive timestamp")
	}
	reported := *input.ReportedCompletedAt
	return Evidence{Source: EvidenceReported, ReportedCompletedAt: &reported}, nil
}

func UsableWatchWorkout(workout *Workout) error {
	if workout == nil || !workout.Present || !workout.WatchVerified {
		return kernel.NewRuleError("unusable_watch_evidence", "watch workout must exist with present = 1 and watch_verified = 1")
	}
	return nil
}

func IsComplete(watchWorkoutID *string, watchPresent bool, reportedCompletedAt *int64) bool {
	if watchWorkoutID != nil && watchPresent {
		return true
	}
	return reportedCompletedAt != nil
}

func NormalizeSets(sessionID string, sets []NewSet) (string, []NewSet, error) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID == "" {
		return "", nil, invalid("session_id is required")
	}
	if len(sets) == 0 {
		return "", nil, invalid("at least one set is required")
	}

	normalized := make([]NewSet, len(sets))
	for index, input := range sets {
		set := input
		set.ExerciseID = strings.TrimSpace(input.ExerciseID)
		if set.ExerciseID == "" {
			return "", nil, invalid("exercise_id is required")
		}
		if set.SetIndex <= 0 {
			return "", nil, invalid("set_index must be greater than zero")
		}
		if set.KG < 0 || math.IsNaN(set.KG) || math.IsInf(set.KG, 0) {
			return "", nil, invalid("kg must be a finite non-negative number")
		}
		if set.Reps <= 0 {
			return "", nil, invalid("reps must be greater than zero")
		}
		set.KG = math.Round(set.KG*100) / 100
		if set.KG == 0 {
			set.KG = 0
		}
		if input.Note != nil {
			note := strings.TrimSpace(*input.Note)
			if note == "" {
				set.Note = nil
			} else {
				set.Note = &note
			}
		}
		normalized[index] = set
	}

	return normalizedSessionID, normalized, nil
}

func EffectiveEvidence(session Session, watchPresent bool) *Evidence {
	if session.WatchWorkoutID != nil && watchPresent {
		return &Evidence{Source: EvidenceWatch, WatchWorkoutID: session.WatchWorkoutID}
	}
	if session.ReportedCompletedAt != nil {
		return &Evidence{Source: EvidenceReported, ReportedCompletedAt: session.ReportedCompletedAt}
	}
	return nil
}

func isISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func invalid(format string, arguments ...any) error {
	return kernel.NewRuleError("invalid_input", fmt.Sprintf(format, arguments...))
}
