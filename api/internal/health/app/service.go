package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
)

const OperationUpsertDay = "day.upsert"

type Receipt struct {
	RequestHash string
	Result      string
}

type DayStore interface {
	GetDay(ctx context.Context, date string) (domain.Day, error)
	SaveDay(ctx context.Context, day domain.Day) error
}

type IdempotencyStore interface {
	Find(ctx context.Context, operation, key string) (Receipt, bool, error)
	Save(ctx context.Context, operation, key, requestHash, result string, createdAt int64) error
}

type Store interface {
	DayStore
	IdempotencyStore
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, days DayStore, idempotency IdempotencyStore) error) error
}

type Clock interface {
	Now() time.Time
}

type UpsertDayCommand struct {
	Date             string
	Timezone         string
	ExpectedRevision int64
	Source           string
	StepsSource      *string
	Metrics          domain.MetricsPatch
	SleepStages      *[]domain.SleepStage
	Workouts         *[]domain.Workout
	IdempotencyKey   string
}

type Service struct {
	store Store
	clock Clock
}

func New(store Store, clock Clock) *Service {
	return &Service{store: store, clock: clock}
}

func (s *Service) GetDay(ctx context.Context, date string) (domain.Day, error) {
	if err := domain.ValidateDate(date); err != nil {
		return domain.Day{}, err
	}
	day, err := s.store.GetDay(ctx, date)
	if err != nil {
		return domain.Day{}, err
	}
	domain.DeriveWorkoutPace(day.Workouts)
	return day, nil
}

func (s *Service) UpsertDay(ctx context.Context, command UpsertDayCommand) (domain.Day, error) {
	if len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 {
		return domain.Day{}, kernel.NewRuleError("idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
	}

	upsert := domain.Upsert{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: command.ExpectedRevision,
		Source:           command.Source,
		StepsSource:      command.StepsSource,
		Metrics:          command.Metrics,
		SleepStages:      command.SleepStages,
		Workouts:         command.Workouts,
	}
	if err := domain.ValidateUpsert(upsert); err != nil {
		return domain.Day{}, err
	}

	hash, err := requestHash(command)
	if err != nil {
		return domain.Day{}, err
	}

	now := s.clock.Now().UnixMilli()
	var result domain.Day

	err = s.store.WithinTransaction(ctx, func(ctx context.Context, days DayStore, idempotency IdempotencyStore) error {
		receipt, found, err := idempotency.Find(ctx, OperationUpsertDay, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if receipt.RequestHash != hash {
				return kernel.NewRuleError("idempotency_conflict", "idempotency key was reused with a different request")
			}
			return json.Unmarshal([]byte(receipt.Result), &result)
		}

		var existing *domain.Day
		stored, err := days.GetDay(ctx, command.Date)
		if err != nil {
			if !errors.Is(err, domain.ErrDayNotFound) {
				return err
			}
		} else {
			existing = &stored
		}

		revision, err := domain.NextRevision(existing, command.ExpectedRevision)
		if err != nil {
			return err
		}

		next := domain.Day{Date: command.Date}
		if existing != nil {
			next = *existing
		}
		next.Date = command.Date
		next.Timezone = command.Timezone
		next.Revision = revision
		next.UpdatedAt = now
		next.LastSource = command.Source
		next.StepsSource = cloneString(command.StepsSource)
		next = domain.ApplyMetrics(next, command.Metrics)

		if command.SleepStages != nil {
			next.SleepStages = cloneStages(*command.SleepStages)
		}
		if command.Workouts != nil {
			reconciled, err := domain.ReconcileWorkouts(next.Workouts, *command.Workouts)
			if err != nil {
				return err
			}
			next.Workouts = reconciled
		}

		if err := days.SaveDay(ctx, next); err != nil {
			return err
		}

		encoded, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if err := idempotency.Save(ctx, OperationUpsertDay, command.IdempotencyKey, hash, string(encoded), now); err != nil {
			return err
		}

		result = next
		return nil
	})
	if err != nil {
		return domain.Day{}, err
	}

	return result, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStages(stages []domain.SleepStage) []domain.SleepStage {
	if stages == nil {
		return nil
	}
	cloned := make([]domain.SleepStage, len(stages))
	copy(cloned, stages)
	return cloned
}

type canonicalCommand struct {
	Date             string              `json:"date"`
	Timezone         string              `json:"timezone"`
	ExpectedRevision int64               `json:"expected_revision"`
	Source           string              `json:"source"`
	StepsSource      *string             `json:"steps_source"`
	Metrics          canonicalMetrics    `json:"metrics"`
	SleepStages      *[]canonicalStage   `json:"sleep_stages"`
	Workouts         *[]canonicalWorkout `json:"workouts"`
}

type canonicalMetrics struct {
	SleepMS      canonicalMetricInt64   `json:"sleep_ms"`
	Steps        canonicalMetricInt64   `json:"steps"`
	RestingHRBPM canonicalMetricInt64   `json:"resting_hr_bpm"`
	HRVRMSSDMS   canonicalMetricFloat64 `json:"hrv_rmssd_ms"`
}

type canonicalMetricInt64 struct {
	Set   bool   `json:"set"`
	Value *int64 `json:"value"`
}

type canonicalMetricFloat64 struct {
	Set   bool     `json:"set"`
	Value *float64 `json:"value"`
}

type canonicalStage struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}

type canonicalWorkout struct {
	Origin        string   `json:"origin"`
	ExternalID    string   `json:"external_id"`
	DeviceKind    *string  `json:"device_kind"`
	StartedAt     int64    `json:"started_at"`
	EndedAt       int64    `json:"ended_at"`
	Discipline    string   `json:"discipline"`
	DurationMS    int64    `json:"duration_ms"`
	DistanceM     *float64 `json:"distance_m"`
	WatchVerified bool     `json:"watch_verified"`
}

func requestHash(command UpsertDayCommand) (string, error) {
	canonical := canonicalCommand{
		Date:             command.Date,
		Timezone:         command.Timezone,
		ExpectedRevision: command.ExpectedRevision,
		Source:           command.Source,
		StepsSource:      command.StepsSource,
		Metrics: canonicalMetrics{
			SleepMS:      canonicalMetricInt64{Set: command.Metrics.SleepMS.Set, Value: command.Metrics.SleepMS.Value},
			Steps:        canonicalMetricInt64{Set: command.Metrics.Steps.Set, Value: command.Metrics.Steps.Value},
			RestingHRBPM: canonicalMetricInt64{Set: command.Metrics.RestingHRBPM.Set, Value: command.Metrics.RestingHRBPM.Value},
			HRVRMSSDMS:   canonicalMetricFloat64{Set: command.Metrics.HRVRMSSDMS.Set, Value: command.Metrics.HRVRMSSDMS.Value},
		},
	}
	if command.SleepStages != nil {
		stages := make([]canonicalStage, len(*command.SleepStages))
		for index, stage := range *command.SleepStages {
			stages[index] = canonicalStage{Stage: stage.Stage, DurationMS: stage.DurationMS}
		}
		canonical.SleepStages = &stages
	}
	if command.Workouts != nil {
		workouts := make([]canonicalWorkout, len(*command.Workouts))
		for index, workout := range *command.Workouts {
			workouts[index] = canonicalWorkout{
				Origin:        workout.Origin,
				ExternalID:    workout.ExternalID,
				DeviceKind:    workout.DeviceKind,
				StartedAt:     workout.StartedAt,
				EndedAt:       workout.EndedAt,
				Discipline:    workout.Discipline,
				DurationMS:    workout.DurationMS,
				DistanceM:     workout.DistanceM,
				WatchVerified: workout.WatchVerified,
			}
		}
		canonical.Workouts = &workouts
	}

	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
