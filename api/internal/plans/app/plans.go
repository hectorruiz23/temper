package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
)

type CreatePlanCommand struct {
	IdempotencyKey string
	Plan           domain.Plan
}

type PlanService struct {
	plans       PlanStore
	idempotency IdempotencyStore
	clock       Clock
}

func NewPlanService(plans PlanStore, idempotency IdempotencyStore, clock Clock) *PlanService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &PlanService{plans: plans, idempotency: idempotency, clock: clock}
}

func (s *PlanService) CreatePlan(ctx context.Context, command CreatePlanCommand) (domain.Plan, error) {
	key := strings.TrimSpace(command.IdempotencyKey)
	if key == "" {
		return domain.Plan{}, kernel.NewRuleError("invalid_input", "idempotency key is required")
	}

	plan, err := domain.ValidatePlan(command.Plan)
	if err != nil {
		return domain.Plan{}, err
	}
	plan.ID = 0
	plan.CreatedAt = 0

	requestHash, err := planHash(plan)
	if err != nil {
		return domain.Plan{}, err
	}
	plan.CreatedAt = s.clock.Now().UTC().UnixMilli()

	payload, err := s.idempotency.Execute(ctx, OperationCreatePlan, key, requestHash, func(ctx context.Context) ([]byte, error) {
		id, err := s.plans.CreatePlan(ctx, plan)
		if err != nil {
			return nil, err
		}
		stored, err := s.plans.Plan(ctx, id)
		if err != nil {
			return nil, err
		}
		return json.Marshal(stored)
	})
	if err != nil {
		return domain.Plan{}, err
	}

	var stored domain.Plan
	if err := json.Unmarshal(payload, &stored); err != nil {
		return domain.Plan{}, err
	}
	return stored, nil
}

func (s *PlanService) GetCurrentPlan(ctx context.Context) (domain.Plan, error) {
	return s.plans.CurrentPlan(ctx)
}

func (s *PlanService) GetPlan(ctx context.Context, id int64) (domain.Plan, error) {
	if id <= 0 {
		return domain.Plan{}, kernel.NewRuleError("not_found", "plan version not found")
	}
	return s.plans.Plan(ctx, id)
}

func planHash(plan domain.Plan) (string, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
