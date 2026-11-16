package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

type CorrectSetCommand struct {
	IdempotencyKey   string
	SetID            string
	ExpectedRevision int64
	Reason           string
	Changes          domain.SetChanges
}

type CorrectionEcho struct {
	ExpectedRevision int64             `json:"expected_revision"`
	Reason           string            `json:"reason"`
	Changes          domain.SetChanges `json:"changes"`
}

type CorrectionResult struct {
	Set          domain.Set     `json:"set"`
	CorrectionID string         `json:"correction_id"`
	Echo         CorrectionEcho `json:"echo"`
}

func (s *Service) CorrectSet(ctx context.Context, command CorrectSetCommand) (CorrectionResult, error) {
	if len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 {
		return CorrectionResult{}, kernel.NewRuleError("idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
	}

	reason, changes, err := domain.NormalizeCorrection(command.Reason, command.Changes)
	if err != nil {
		return CorrectionResult{}, err
	}

	setID := strings.TrimSpace(command.SetID)
	hash, err := correctionHash(setID, command.ExpectedRevision, reason, changes)
	if err != nil {
		return CorrectionResult{}, err
	}

	var result CorrectionResult
	err = s.store.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		receipt, found, err := tx.FindReceipt(ctx, OperationCorrectSet, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if receipt.RequestHash != hash {
				return kernel.NewRuleError("idempotency_conflict", "idempotency key was reused with a different request")
			}
			return json.Unmarshal([]byte(receipt.Result), &result)
		}

		set, err := tx.Set(ctx, setID)
		if err != nil {
			return err
		}
		if set.Revision != command.ExpectedRevision {
			return kernel.NewRuleError("revision_conflict", "expected_revision does not match the stored revision")
		}

		updated, err := domain.ApplyCorrection(set, changes)
		if err != nil {
			return err
		}

		exists, err := tx.ExerciseExists(ctx, updated.ExerciseID)
		if err != nil {
			return err
		}
		if !exists {
			return kernel.NewRuleError("unknown_exercise", "exercise does not exist")
		}

		required, stock, err := tx.Equipment(ctx, updated.ExerciseID)
		if err != nil {
			return err
		}
		if err := domain.ValidateEquipmentLimit(updated.ExerciseID, updated.KG, required, stock); err != nil {
			return err
		}

		if err := tx.UpdateSet(ctx, updated, set.Revision); err != nil {
			return err
		}

		beforeJSON, err := domain.CanonicalSetJSON(set)
		if err != nil {
			return err
		}
		afterJSON, err := domain.CanonicalSetJSON(updated)
		if err != nil {
			return err
		}

		correctionID, err := newID()
		if err != nil {
			return err
		}
		key := command.IdempotencyKey
		correction := domain.SetCorrection{
			ID:                correctionID,
			SetID:             set.ID,
			CreatedAt:         s.clock.Now().UnixMilli(),
			Reason:            reason,
			BeforeJSON:        beforeJSON,
			AfterJSON:         afterJSON,
			ResultingRevision: updated.Revision,
			IdempotencyKey:    &key,
		}
		if err := tx.InsertCorrection(ctx, correction); err != nil {
			return err
		}

		result = CorrectionResult{
			Set:          updated,
			CorrectionID: correctionID,
			Echo: CorrectionEcho{
				ExpectedRevision: command.ExpectedRevision,
				Reason:           reason,
				Changes:          changes,
			},
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReceipt(ctx, OperationCorrectSet, command.IdempotencyKey, hash, string(encoded), correction.CreatedAt)
	})
	if err != nil {
		return CorrectionResult{}, err
	}

	return result, nil
}

func (s *Service) Progression(ctx context.Context, exerciseID string) (domain.Progression, error) {
	id := strings.TrimSpace(exerciseID)
	var result domain.Progression
	err := s.store.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		facts, exists, err := tx.ExerciseFacts(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return kernel.NewRuleError("not_found", "exercise not found")
		}

		history, err := tx.PerformedSets(ctx, id)
		if err != nil {
			return err
		}
		targets, err := tx.CurrentTargets(ctx, id)
		if err != nil {
			return err
		}

		result = domain.Progression{
			PerformedHistory:      history,
			CurrentTargets:        targets,
			ProgressionDimensions: domain.ProgressionDimensions(facts.Unilateral, targets),
		}
		return nil
	})
	if err != nil {
		return domain.Progression{}, err
	}

	return result, nil
}

type correctionCanonical struct {
	SetID            string            `json:"set_id"`
	ExpectedRevision int64             `json:"expected_revision"`
	Reason           string            `json:"reason"`
	Changes          domain.SetChanges `json:"changes"`
}

func correctionHash(setID string, expectedRevision int64, reason string, changes domain.SetChanges) (string, error) {
	return hashJSON(correctionCanonical{
		SetID:            setID,
		ExpectedRevision: expectedRevision,
		Reason:           reason,
		Changes:          changes,
	})
}
