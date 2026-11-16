package domain

import "fmt"

const (
	ProgressionBasisPlan      = "session_exercise"
	ProgressionBasisCatalogue = "exercise"
)

type ExerciseFacts struct {
	ID         string
	Unilateral bool
}

type PerformedSet struct {
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

type ExerciseTarget struct {
	TemplateID string   `json:"template_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type ProgressionDimension struct {
	Name      string `json:"name"`
	Dimension any    `json:"dimension"`
	Basis     string `json:"basis"`
}

type Progression struct {
	PerformedHistory      []PerformedSet         `json:"performed_history"`
	CurrentTargets        []ExerciseTarget       `json:"current_targets"`
	ProgressionDimensions []ProgressionDimension `json:"progression_dimensions"`
}

func ProgressionDimensions(unilateral bool, targets []ExerciseTarget) []ProgressionDimension {
	dimensions := make([]ProgressionDimension, 0, 4)

	var target *ExerciseTarget
	if len(targets) > 0 {
		target = &targets[0]
	}

	if target != nil && target.TargetSets != nil {
		dimensions = append(dimensions, ProgressionDimension{Name: "sets", Dimension: *target.TargetSets, Basis: ProgressionBasisPlan})
	}
	if target != nil && (target.RepsMin != nil || target.RepsMax != nil) {
		dimensions = append(dimensions, ProgressionDimension{Name: "reps", Dimension: repsDimension(target), Basis: ProgressionBasisPlan})
	}
	if target != nil && target.Tempo != nil {
		dimensions = append(dimensions, ProgressionDimension{Name: "tempo", Dimension: *target.Tempo, Basis: ProgressionBasisPlan})
	}
	if unilateral {
		dimensions = append(dimensions, ProgressionDimension{Name: "unilateral", Dimension: true, Basis: ProgressionBasisCatalogue})
	}

	return dimensions
}

func repsDimension(target *ExerciseTarget) string {
	switch {
	case target.RepsMin != nil && target.RepsMax != nil:
		return fmt.Sprintf("%d-%d", *target.RepsMin, *target.RepsMax)
	case target.RepsMin != nil:
		return fmt.Sprintf("%d", *target.RepsMin)
	default:
		return fmt.Sprintf("%d", *target.RepsMax)
	}
}
