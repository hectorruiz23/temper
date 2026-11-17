package app

import (
	"context"

	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

func (s *Service) SessionsBetween(ctx context.Context, startDate, endDate string) ([]domain.Session, error) {
	return s.store.SessionsBetween(ctx, startDate, endDate)
}
