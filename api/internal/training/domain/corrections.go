package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/kernel"
)

type SetChanges struct {
	ExerciseID *string  `json:"exercise_id,omitempty"`
	SetIndex   *int     `json:"set_index,omitempty"`
	KG         *float64 `json:"kg,omitempty"`
	Reps       *int     `json:"reps,omitempty"`
	Note       *string  `json:"note,omitempty"`
}

type SetCorrection struct {
	ID                string
	SetID             string
	CreatedAt         int64
	Reason            string
	BeforeJSON        string
	AfterJSON         string
	ResultingRevision int64
	IdempotencyKey    *string
}

func (c SetChanges) Empty() bool {
	return c.ExerciseID == nil && c.SetIndex == nil && c.KG == nil && c.Reps == nil && c.Note == nil
}

func NormalizeCorrection(reason string, changes SetChanges) (string, SetChanges, error) {
	normalizedReason := strings.TrimSpace(reason)
	if normalizedReason == "" {
		return "", SetChanges{}, invalid("reason is required")
	}
	if changes.Empty() {
		return "", SetChanges{}, invalid("at least one change is required")
	}

	normalized := changes
	if changes.ExerciseID != nil {
		exerciseID := strings.TrimSpace(*changes.ExerciseID)
		if exerciseID == "" {
			return "", SetChanges{}, invalid("exercise_id must not be empty")
		}
		normalized.ExerciseID = &exerciseID
	}
	if changes.SetIndex != nil {
		if *changes.SetIndex <= 0 {
			return "", SetChanges{}, invalid("set_index must be greater than zero")
		}
		setIndex := *changes.SetIndex
		normalized.SetIndex = &setIndex
	}
	if changes.KG != nil {
		if *changes.KG < 0 || math.IsNaN(*changes.KG) || math.IsInf(*changes.KG, 0) {
			return "", SetChanges{}, invalid("kg must be a finite non-negative number")
		}
		kg := math.Round(*changes.KG*100) / 100
		if kg == 0 {
			kg = 0
		}
		normalized.KG = &kg
	}
	if changes.Reps != nil {
		if *changes.Reps <= 0 {
			return "", SetChanges{}, invalid("reps must be greater than zero")
		}
		reps := *changes.Reps
		normalized.Reps = &reps
	}
	if changes.Note != nil {
		note := strings.TrimSpace(*changes.Note)
		normalized.Note = &note
	}

	return normalizedReason, normalized, nil
}

func ApplyCorrection(set Set, changes SetChanges) (Set, error) {
	updated := set
	if changes.ExerciseID != nil {
		updated.ExerciseID = *changes.ExerciseID
	}
	if changes.SetIndex != nil {
		updated.SetIndex = *changes.SetIndex
	}
	if changes.KG != nil {
		updated.KG = *changes.KG
	}
	if changes.Reps != nil {
		updated.Reps = *changes.Reps
	}
	if changes.Note != nil {
		note := strings.TrimSpace(*changes.Note)
		if note == "" {
			updated.Note = nil
		} else {
			updated.Note = &note
		}
	}
	updated.Revision = set.Revision + 1

	if strings.TrimSpace(updated.ExerciseID) == "" {
		return Set{}, invalid("exercise_id is required")
	}
	if updated.SetIndex <= 0 {
		return Set{}, invalid("set_index must be greater than zero")
	}
	if updated.KG < 0 || math.IsNaN(updated.KG) || math.IsInf(updated.KG, 0) {
		return Set{}, invalid("kg must be a finite non-negative number")
	}
	if updated.Reps <= 0 {
		return Set{}, invalid("reps must be greater than zero")
	}
	updated.KG = math.Round(updated.KG*100) / 100
	if updated.KG == 0 {
		updated.KG = 0
	}

	return updated, nil
}

func CanonicalSetJSON(set Set) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"id":              set.ID,
		"session_id":      set.SessionID,
		"exercise_id":     set.ExerciseID,
		"set_index":       set.SetIndex,
		"kg":              set.KG,
		"reps":            set.Reps,
		"note":            set.Note,
		"evidence_source": set.EvidenceSource,
		"idempotency_key": set.IdempotencyKey,
		"created_at":      set.CreatedAt,
		"revision":        set.Revision,
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

type EquipmentRequirement struct {
	EquipmentID string
	Quantity    int
}

type InventoryStock struct {
	EquipmentID string
	Quantity    int
	MaxKG       *float64
}

func ValidateEquipmentLimit(exerciseID string, kg float64, required []EquipmentRequirement, stock []InventoryStock) error {
	available := make(map[string]InventoryStock, len(stock))
	for _, item := range stock {
		available[item.EquipmentID] = item
	}

	for _, requirement := range required {
		item, ok := available[requirement.EquipmentID]
		if !ok || item.MaxKG == nil {
			continue
		}
		if kg > *item.MaxKG {
			return kernel.NewRuleError("equipment_limit", fmt.Sprintf("exercise %q uses %g kg but %q allows at most %g kg", exerciseID, kg, requirement.EquipmentID, *item.MaxKG))
		}
	}

	return nil
}
