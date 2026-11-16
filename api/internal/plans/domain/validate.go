package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

func ValidatePlan(plan Plan) (Plan, error) {
	normalized := plan
	normalized.Reason = strings.TrimSpace(plan.Reason)
	normalized.EffectiveFrom = strings.TrimSpace(plan.EffectiveFrom)
	normalized.Timezone = strings.TrimSpace(plan.Timezone)
	normalized.GoalID = strings.TrimSpace(plan.GoalID)
	normalized.Sessions = make([]Session, len(plan.Sessions))

	if normalized.Reason == "" {
		return Plan{}, kernel.NewRuleError("invalid_input", "plan reason is required")
	}
	if normalized.WeeklyFrequency <= 0 {
		return Plan{}, kernel.NewRuleError("invalid_input", "weekly frequency must be greater than zero")
	}
	if !isISODate(normalized.EffectiveFrom) {
		return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("effective_from %q is not an ISO date", normalized.EffectiveFrom))
	}
	if err := validateTimezone(normalized.Timezone); err != nil {
		return Plan{}, err
	}

	sessionPositions := make(map[int]bool, len(plan.Sessions))
	sessionIDs := make(map[string]bool, len(plan.Sessions))

	for index, input := range plan.Sessions {
		session := input
		session.ID = strings.TrimSpace(input.ID)
		session.Target = strings.TrimSpace(input.Target)

		if session.ID == "" {
			return Plan{}, kernel.NewRuleError("invalid_input", "session id is required")
		}
		if sessionIDs[session.ID] {
			return Plan{}, kernel.NewRuleError("invalid_input", fmt.Sprintf("session id %q is duplicated", session.ID))
		}
		sessionIDs[session.ID] = true

		if session.Position < 0 {
			return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q has a negative position", session.ID))
		}
		if sessionPositions[session.Position] {
			return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session position %d is duplicated", session.Position))
		}
		sessionPositions[session.Position] = true

		switch session.Discipline {
		case DisciplineStrength, DisciplineRun, DisciplineIntervals:
		default:
			return Plan{}, kernel.NewRuleError("invalid_input", fmt.Sprintf("session %q has unknown discipline %q", session.ID, session.Discipline))
		}

		if (session.ScheduledDate == nil) == (session.Weekday == nil) {
			return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q must have exactly one of scheduled_date and weekday", session.ID))
		}
		if session.ScheduledDate != nil {
			date := strings.TrimSpace(*session.ScheduledDate)
			if !isISODate(date) {
				return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q scheduled_date %q is not an ISO date", session.ID, date))
			}
			session.ScheduledDate = &date
		}
		if session.Weekday != nil && (*session.Weekday < 1 || *session.Weekday > 7) {
			return Plan{}, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q weekday must be between 1 and 7", session.ID))
		}

		exercises, err := validateSessionExercises(session)
		if err != nil {
			return Plan{}, err
		}
		session.Exercises = exercises

		steps, err := validateIntervalSteps(session)
		if err != nil {
			return Plan{}, err
		}
		session.Steps = steps

		normalized.Sessions[index] = session
	}

	sort.SliceStable(normalized.Sessions, func(i, j int) bool {
		return normalized.Sessions[i].Position < normalized.Sessions[j].Position
	})

	return normalized, nil
}

func validateSessionExercises(session Session) ([]SessionExercise, error) {
	normalized := make([]SessionExercise, len(session.Exercises))
	positions := make(map[int]bool, len(session.Exercises))

	for index, input := range session.Exercises {
		exercise := input
		exercise.ExerciseID = strings.TrimSpace(input.ExerciseID)

		if exercise.ExerciseID == "" {
			return nil, kernel.NewRuleError("invalid_input", fmt.Sprintf("session %q has an exercise without an exercise id", session.ID))
		}
		if exercise.Position < 0 {
			return nil, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q has a negative exercise position", session.ID))
		}
		if positions[exercise.Position] {
			return nil, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q has duplicated exercise position %d", session.ID, exercise.Position))
		}
		positions[exercise.Position] = true

		if exercise.TargetSets != nil && *exercise.TargetSets <= 0 {
			return nil, kernel.NewRuleError("invalid_target", fmt.Sprintf("exercise %q target_sets must be greater than zero", exercise.ExerciseID))
		}
		if exercise.RepsMin != nil && *exercise.RepsMin < 0 {
			return nil, kernel.NewRuleError("invalid_target", fmt.Sprintf("exercise %q reps_min must not be negative", exercise.ExerciseID))
		}
		if exercise.RepsMax != nil && *exercise.RepsMax < 0 {
			return nil, kernel.NewRuleError("invalid_target", fmt.Sprintf("exercise %q reps_max must not be negative", exercise.ExerciseID))
		}
		if exercise.RepsMin != nil && exercise.RepsMax != nil && *exercise.RepsMin > *exercise.RepsMax {
			return nil, kernel.NewRuleError("invalid_target", fmt.Sprintf("exercise %q reps_min is greater than reps_max", exercise.ExerciseID))
		}
		if exercise.TargetKG != nil && *exercise.TargetKG < 0 {
			return nil, kernel.NewRuleError("invalid_target", fmt.Sprintf("exercise %q target_kg must not be negative", exercise.ExerciseID))
		}
		if exercise.Tempo != nil {
			tempo := strings.TrimSpace(*exercise.Tempo)
			exercise.Tempo = &tempo
		}

		normalized[index] = exercise
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Position < normalized[j].Position
	})

	return normalized, nil
}

func validateIntervalSteps(session Session) ([]IntervalStep, error) {
	if session.Discipline != DisciplineIntervals {
		if len(session.Steps) > 0 {
			return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("session %q is not an intervals session but has interval steps", session.ID))
		}
		return nil, nil
	}
	if len(session.Steps) == 0 {
		return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("session %q is an intervals session without steps", session.ID))
	}

	normalized := make([]IntervalStep, len(session.Steps))
	positions := make(map[int]bool, len(session.Steps))

	for index, input := range session.Steps {
		step := input
		step.Label = strings.TrimSpace(input.Label)

		if step.Label == "" {
			return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("session %q has an interval step without a label", session.ID))
		}
		if step.Position < 0 {
			return nil, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q has a negative interval step position", session.ID))
		}
		if positions[step.Position] {
			return nil, kernel.NewRuleError("invalid_schedule", fmt.Sprintf("session %q has duplicated interval step position %d", session.ID, step.Position))
		}
		positions[step.Position] = true

		if (step.DurationMS == nil) == (step.DistanceM == nil) {
			return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("interval step %q must have exactly one of duration_ms and distance_m", step.Label))
		}
		if step.DurationMS != nil && *step.DurationMS <= 0 {
			return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("interval step %q duration_ms must be greater than zero", step.Label))
		}
		if step.DistanceM != nil && *step.DistanceM <= 0 {
			return nil, kernel.NewRuleError("invalid_interval_steps", fmt.Sprintf("interval step %q distance_m must be greater than zero", step.Label))
		}

		normalized[index] = step
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Position < normalized[j].Position
	})

	return normalized, nil
}

func ValidateExercise(exercise Exercise) (Exercise, error) {
	normalized := exercise
	normalized.ID = strings.TrimSpace(exercise.ID)
	normalized.Name = strings.TrimSpace(exercise.Name)
	normalized.MovementPattern = strings.TrimSpace(exercise.MovementPattern)
	normalized.Equipment = make([]ExerciseEquipment, len(exercise.Equipment))

	if normalized.ID == "" {
		return Exercise{}, kernel.NewRuleError("invalid_input", "exercise id is required")
	}
	if normalized.Name == "" {
		return Exercise{}, kernel.NewRuleError("invalid_input", "exercise name is required")
	}
	if normalized.MovementPattern == "" {
		return Exercise{}, kernel.NewRuleError("invalid_input", "exercise movement pattern is required")
	}

	seen := make(map[string]bool, len(exercise.Equipment))
	for index, input := range exercise.Equipment {
		equipment := input
		equipment.EquipmentID = strings.TrimSpace(input.EquipmentID)
		if equipment.EquipmentID == "" {
			return Exercise{}, kernel.NewRuleError("invalid_input", "exercise equipment id is required")
		}
		if seen[equipment.EquipmentID] {
			return Exercise{}, kernel.NewRuleError("invalid_input", fmt.Sprintf("equipment %q is duplicated", equipment.EquipmentID))
		}
		seen[equipment.EquipmentID] = true
		if equipment.Quantity <= 0 {
			return Exercise{}, kernel.NewRuleError("invalid_input", fmt.Sprintf("equipment %q quantity must be greater than zero", equipment.EquipmentID))
		}
		normalized.Equipment[index] = equipment
	}

	sort.SliceStable(normalized.Equipment, func(i, j int) bool {
		return normalized.Equipment[i].EquipmentID < normalized.Equipment[j].EquipmentID
	})

	return normalized, nil
}

func CheckEquipment(exerciseID string, required []EquipmentRequirement, stock []InventoryStock, targetKG *float64) error {
	available := make(map[string]InventoryStock, len(stock))
	for _, item := range stock {
		available[item.EquipmentID] = item
	}

	for _, requirement := range required {
		item, ok := available[requirement.EquipmentID]
		if !ok {
			return kernel.NewRuleError("unavailable_equipment", fmt.Sprintf("exercise %q requires equipment %q that the inventory does not hold", exerciseID, requirement.EquipmentID))
		}
		if item.Quantity < requirement.Quantity {
			return kernel.NewRuleError("unavailable_equipment", fmt.Sprintf("exercise %q requires %d of %q but the inventory holds %d", exerciseID, requirement.Quantity, requirement.EquipmentID, item.Quantity))
		}
		if targetKG != nil && item.MaxKG != nil && *targetKG > *item.MaxKG {
			return kernel.NewRuleError("equipment_limit", fmt.Sprintf("exercise %q targets %g kg but %q allows at most %g kg", exerciseID, *targetKG, requirement.EquipmentID, *item.MaxKG))
		}
	}

	return nil
}

func isISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validateTimezone(name string) error {
	if name == "" {
		return kernel.NewRuleError("invalid_schedule", "timezone is required")
	}
	if name == "Local" {
		return kernel.NewRuleError("invalid_schedule", "timezone must be a known IANA name")
	}
	if _, err := time.LoadLocation(name); err != nil {
		return kernel.NewRuleError("invalid_schedule", fmt.Sprintf("timezone %q is not a known IANA name", name))
	}
	return nil
}
