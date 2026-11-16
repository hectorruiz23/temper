package app

import (
	"context"
	"time"

	"github.com/hectorruiz23/temper/api/internal/plans/domain"
)

const OperationCreatePlan = "plans.create"

type Clock interface {
	Now() time.Time
}

type PlanStore interface {
	CreatePlan(ctx context.Context, plan domain.Plan) (int64, error)
	CurrentPlan(ctx context.Context) (domain.Plan, error)
	Plan(ctx context.Context, id int64) (domain.Plan, error)
	PlanEffectiveOn(ctx context.Context, date string) (domain.Plan, error)
}

type CatalogueStore interface {
	CreateExercise(ctx context.Context, exercise domain.Exercise) error
	ListExercises(ctx context.Context) ([]domain.Exercise, error)
	Inventory(ctx context.Context) ([]domain.InventoryItem, error)
}

type IdempotencyStore interface {
	Execute(ctx context.Context, operation, key, requestHash string, mutate func(ctx context.Context) ([]byte, error)) ([]byte, error)
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}
