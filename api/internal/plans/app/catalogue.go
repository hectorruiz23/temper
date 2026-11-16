package app

import (
	"context"

	"github.com/hectorruiz23/temper/api/internal/plans/domain"
)

type CreateExerciseCommand struct {
	ID              string
	Name            string
	MovementPattern string
	Unilateral      bool
	Equipment       []domain.ExerciseEquipment
}

type CatalogueService struct {
	catalogue CatalogueStore
	clock     Clock
}

func NewCatalogueService(catalogue CatalogueStore, clock Clock) *CatalogueService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &CatalogueService{catalogue: catalogue, clock: clock}
}

func (s *CatalogueService) CreateExercise(ctx context.Context, command CreateExerciseCommand) (domain.Exercise, error) {
	exercise, err := domain.ValidateExercise(domain.Exercise{
		ID:              command.ID,
		Name:            command.Name,
		MovementPattern: command.MovementPattern,
		Unilateral:      command.Unilateral,
		Equipment:       command.Equipment,
	})
	if err != nil {
		return domain.Exercise{}, err
	}
	exercise.CreatedAt = s.clock.Now().UTC().UnixMilli()

	if err := s.catalogue.CreateExercise(ctx, exercise); err != nil {
		return domain.Exercise{}, err
	}
	return exercise, nil
}

func (s *CatalogueService) ListExercises(ctx context.Context) ([]domain.Exercise, error) {
	return s.catalogue.ListExercises(ctx)
}

func (s *CatalogueService) GetInventory(ctx context.Context) ([]domain.InventoryItem, error) {
	return s.catalogue.Inventory(ctx)
}
