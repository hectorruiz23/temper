package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

type CreateSessionCommand struct {
	IdempotencyKey string
	ID             string
	TemplateID     *string
	Date           string
	Discipline     string
	StartedAt      *int64
}

type CheckInCommand struct {
	SessionID           string
	ExpectedRevision    int64
	ReportedCompletedAt *int64
	WatchWorkoutID      *string
}

type CheckInResult struct {
	Session  domain.Session
	Evidence *domain.Evidence
}

type LogSetsCommand struct {
	IdempotencyKey string
	SessionID      string
	Sets           []domain.NewSet
}

type BatchEcho struct {
	SessionID string          `json:"session_id"`
	Sets      []domain.NewSet `json:"sets"`
}

type BatchResult struct {
	Sets []domain.Set `json:"sets"`
	Echo BatchEcho    `json:"echo"`
}

type Service struct {
	store Store
	clock Clock
}

func New(store Store, clock Clock) *Service {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}
}

func (s *Service) CreateSession(ctx context.Context, command CreateSessionCommand) (domain.Session, error) {
	if len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 {
		return domain.Session{}, kernel.NewRuleError("idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
	}

	input, err := domain.ValidateNewSession(domain.NewSession{
		ID:         command.ID,
		TemplateID: command.TemplateID,
		Date:       command.Date,
		Discipline: command.Discipline,
		StartedAt:  command.StartedAt,
	})
	if err != nil {
		return domain.Session{}, err
	}

	hash, err := sessionHash(input)
	if err != nil {
		return domain.Session{}, err
	}

	var result domain.Session
	var sessionPlanVersion *int64
	err = s.store.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		receipt, found, err := tx.FindReceipt(ctx, OperationCreateSession, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if receipt.RequestHash != hash {
				return kernel.NewRuleError("idempotency_conflict", "idempotency key was reused with a different request")
			}
			return json.Unmarshal([]byte(receipt.Result), &result)
		}

		if input.TemplateID != nil {
			templateDiscipline, planVersionID, exists, err := tx.Template(ctx, *input.TemplateID, input.Date)
			if err != nil {
				return err
			}
			if !exists {
				return kernel.NewRuleError("unknown_template", "session template does not exist")
			}
			if err := domain.ValidateTemplateDiscipline(input.Discipline, templateDiscipline); err != nil {
				return err
			}
			planVersion := planVersionID
			sessionPlanVersion = &planVersion
		}

		session := domain.Session{
			ID:            input.ID,
			PlanVersionID: sessionPlanVersion,
			TemplateID:    input.TemplateID,
			Date:          input.Date,
			Discipline:    input.Discipline,
			StartedAt:     input.StartedAt,
			CreatedAt:     s.clock.Now().UnixMilli(),
			Revision:      1,
			Complete:      false,
		}
		if err := tx.CreateSession(ctx, session); err != nil {
			return err
		}

		encoded, err := json.Marshal(session)
		if err != nil {
			return err
		}
		if err := tx.SaveReceipt(ctx, OperationCreateSession, command.IdempotencyKey, hash, string(encoded), session.CreatedAt); err != nil {
			return err
		}

		result = session
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}

	return result, nil
}

func (s *Service) CheckIn(ctx context.Context, command CheckInCommand) (CheckInResult, error) {
	evidence, err := domain.ValidateCheckIn(domain.CheckInInput{
		ExpectedRevision:    command.ExpectedRevision,
		ReportedCompletedAt: command.ReportedCompletedAt,
		WatchWorkoutID:      command.WatchWorkoutID,
	})
	if err != nil {
		return CheckInResult{}, err
	}

	var result CheckInResult
	err = s.store.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		session, err := tx.Session(ctx, command.SessionID)
		if err != nil {
			return err
		}
		if session.Revision != command.ExpectedRevision {
			return kernel.NewRuleError("revision_conflict", "expected_revision does not match the stored revision")
		}

		watchPresent := false
		var nextWatch *string
		var nextReported *int64
		if evidence.Source == domain.EvidenceWatch {
			workout, err := tx.Workout(ctx, *evidence.WatchWorkoutID)
			if err != nil {
				return err
			}
			if err := domain.UsableWatchWorkout(workout); err != nil {
				return err
			}
			watchPresent = true
			nextWatch = evidence.WatchWorkoutID
		} else {
			nextReported = evidence.ReportedCompletedAt
		}

		nextRevision := session.Revision + 1
		if err := tx.UpdateSessionEvidence(ctx, session.ID, session.Revision, nextRevision, nextWatch, nextReported); err != nil {
			return err
		}

		session.WatchWorkoutID = nextWatch
		session.ReportedCompletedAt = nextReported
		session.Revision = nextRevision
		session.Complete = domain.IsComplete(nextWatch, watchPresent, nextReported)

		result = CheckInResult{Session: session, Evidence: domain.EffectiveEvidence(session, watchPresent)}
		return nil
	})
	if err != nil {
		return CheckInResult{}, err
	}

	return result, nil
}

func (s *Service) LogSets(ctx context.Context, command LogSetsCommand) (BatchResult, error) {
	if len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 {
		return BatchResult{}, kernel.NewRuleError("idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
	}

	sessionID, normalized, err := domain.NormalizeSets(command.SessionID, command.Sets)
	if err != nil {
		return BatchResult{}, err
	}

	hash, err := setsHash(sessionID, normalized)
	if err != nil {
		return BatchResult{}, err
	}

	var result BatchResult
	err = s.store.WithinTransaction(ctx, func(ctx context.Context, tx Transaction) error {
		receipt, found, err := tx.FindReceipt(ctx, OperationLogSets, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if receipt.RequestHash != hash {
				return kernel.NewRuleError("idempotency_conflict", "idempotency key was reused with a different request")
			}
			return json.Unmarshal([]byte(receipt.Result), &result)
		}

		session, err := tx.Session(ctx, sessionID)
		if err != nil {
			return err
		}

		source := domain.EvidenceReported
		if session.WatchWorkoutID != nil {
			workout, err := tx.Workout(ctx, *session.WatchWorkoutID)
			if err != nil {
				return err
			}
			if workout != nil && workout.Present {
				source = domain.EvidenceWatch
			}
		}

		now := s.clock.Now().UnixMilli()
		sets := make([]domain.Set, 0, len(normalized))
		known := make(map[string]bool, len(normalized))
		for _, input := range normalized {
			exists, ok := known[input.ExerciseID]
			if !ok {
				exists, err = tx.ExerciseExists(ctx, input.ExerciseID)
				if err != nil {
					return err
				}
				known[input.ExerciseID] = exists
			}
			if !exists {
				return kernel.NewRuleError("unknown_exercise", "exercise does not exist")
			}

			id, err := newID()
			if err != nil {
				return err
			}
			key := command.IdempotencyKey
			set := domain.Set{
				ID:             id,
				SessionID:      sessionID,
				ExerciseID:     input.ExerciseID,
				SetIndex:       input.SetIndex,
				KG:             input.KG,
				Reps:           input.Reps,
				Note:           input.Note,
				EvidenceSource: source,
				IdempotencyKey: &key,
				CreatedAt:      now,
				Revision:       1,
			}
			if err := tx.InsertSet(ctx, set); err != nil {
				return err
			}
			sets = append(sets, set)
		}

		result = BatchResult{Sets: sets, Echo: BatchEcho{SessionID: sessionID, Sets: normalized}}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReceipt(ctx, OperationLogSets, command.IdempotencyKey, hash, string(encoded), now)
	})
	if err != nil {
		return BatchResult{}, err
	}

	return result, nil
}

type sessionCanonical struct {
	ID         string  `json:"id"`
	TemplateID *string `json:"template_id"`
	Date       string  `json:"date"`
	Discipline string  `json:"discipline"`
	StartedAt  *int64  `json:"started_at"`
}

func sessionHash(input domain.NewSession) (string, error) {
	return hashJSON(sessionCanonical{
		ID:         input.ID,
		TemplateID: input.TemplateID,
		Date:       input.Date,
		Discipline: input.Discipline,
		StartedAt:  input.StartedAt,
	})
}

type setsCanonical struct {
	SessionID string          `json:"session_id"`
	Sets      []domain.NewSet `json:"sets"`
}

func setsHash(sessionID string, sets []domain.NewSet) (string, error) {
	return hashJSON(setsCanonical{SessionID: sessionID, Sets: sets})
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
