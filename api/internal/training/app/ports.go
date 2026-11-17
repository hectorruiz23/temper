package app

import (
	"context"
	"time"

	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

const (
	OperationCreateSession = "sessions.create"
	OperationLogSets       = "sets.log"
	OperationCorrectSet    = "sets.correct"
)

type Receipt struct {
	RequestHash string
	Result      string
}

type Store interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx Transaction) error) error
	SessionsBetween(ctx context.Context, startDate, endDate string) ([]domain.Session, error)
}

type Transaction interface {
	FindReceipt(ctx context.Context, operation, key string) (Receipt, bool, error)
	SaveReceipt(ctx context.Context, operation, key, requestHash, result string, createdAt int64) error
	Template(ctx context.Context, id, date string) (string, int64, bool, error)
	CreateSession(ctx context.Context, session domain.Session) error
	Session(ctx context.Context, id string) (domain.Session, error)
	UpdateSessionEvidence(ctx context.Context, id string, expectedRevision, nextRevision int64, watchWorkoutID *string, reportedCompletedAt *int64) error
	Workout(ctx context.Context, id string) (*domain.Workout, error)
	ExerciseExists(ctx context.Context, id string) (bool, error)
	InsertSet(ctx context.Context, set domain.Set) error
	Set(ctx context.Context, id string) (domain.Set, error)
	UpdateSet(ctx context.Context, set domain.Set, expectedRevision int64) error
	InsertCorrection(ctx context.Context, correction domain.SetCorrection) error
	Equipment(ctx context.Context, exerciseID string) ([]domain.EquipmentRequirement, []domain.InventoryStock, error)
	ExerciseFacts(ctx context.Context, id string) (domain.ExerciseFacts, bool, error)
	PerformedSets(ctx context.Context, exerciseID string) ([]domain.PerformedSet, error)
	CurrentTargets(ctx context.Context, exerciseID string) ([]domain.ExerciseTarget, error)
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}
