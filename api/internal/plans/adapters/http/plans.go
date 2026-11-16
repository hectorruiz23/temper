package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/plans/domain"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
)

const maxBodyBytes = 64 << 10

var errInvalidBody = errors.New("invalid request body")

type PlanHandler struct {
	service *app.PlanService
}

func NewPlanHandler(service *app.PlanService) *PlanHandler {
	return &PlanHandler{service: service}
}

func (h *PlanHandler) Create(w nethttp.ResponseWriter, request *nethttp.Request) {
	key := request.Header.Get("Idempotency-Key")
	if len(key) == 0 || len(key) > 128 {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
		return
	}

	var input planRequest
	if !readJSONBody(w, request, &input) {
		return
	}

	plan, err := h.service.CreatePlan(request.Context(), app.CreatePlanCommand{
		IdempotencyKey: key,
		Plan:           input.domainPlan(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusCreated, planEnvelope{Plan: planResponseFrom(plan)})
}

func (h *PlanHandler) Current(w nethttp.ResponseWriter, request *nethttp.Request) {
	plan, err := h.service.GetCurrentPlan(request.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, planEnvelope{Plan: planResponseFrom(plan)})
}

func (h *PlanHandler) Get(w nethttp.ResponseWriter, request *nethttp.Request) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusNotFound, "not_found", "plan version not found")
		return
	}

	plan, err := h.service.GetPlan(request.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, planEnvelope{Plan: planResponseFrom(plan)})
}

func readJSONBody(w nethttp.ResponseWriter, request *nethttp.Request, target any) bool {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return false
	}
	if len(body) > maxBodyBytes {
		httpserver.WriteError(w, nethttp.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 64 KiB")
		return false
	}
	if err := decodeJSONBody(body, target); err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return false
	}
	return true
}

func decodeJSONBody(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errInvalidBody
		}
		return err
	}

	return nil
}

type planRequest struct {
	Reason          string           `json:"reason"`
	EffectiveFrom   string           `json:"effective_from"`
	Timezone        string           `json:"timezone"`
	WeeklyFrequency int              `json:"weekly_frequency"`
	GoalID          string           `json:"goal_id"`
	Sessions        []sessionRequest `json:"sessions"`
}

type sessionRequest struct {
	ID            string                   `json:"id"`
	Position      int                      `json:"position"`
	ScheduledDate *string                  `json:"scheduled_date"`
	Weekday       *int                     `json:"weekday"`
	Discipline    string                   `json:"discipline"`
	Target        string                   `json:"target"`
	Exercises     []sessionExerciseRequest `json:"exercises"`
	IntervalSteps []intervalStepRequest    `json:"interval_steps"`
}

type sessionExerciseRequest struct {
	Position   int      `json:"position"`
	ExerciseID string   `json:"exercise_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type intervalStepRequest struct {
	Position   int      `json:"position"`
	Label      string   `json:"label"`
	DurationMS *int     `json:"duration_ms"`
	DistanceM  *float64 `json:"distance_m"`
}

func (input planRequest) domainPlan() domain.Plan {
	sessions := make([]domain.Session, len(input.Sessions))
	for index, session := range input.Sessions {
		exercises := make([]domain.SessionExercise, len(session.Exercises))
		for exerciseIndex, exercise := range session.Exercises {
			exercises[exerciseIndex] = domain.SessionExercise{
				Position:   exercise.Position,
				ExerciseID: exercise.ExerciseID,
				TargetSets: exercise.TargetSets,
				RepsMin:    exercise.RepsMin,
				RepsMax:    exercise.RepsMax,
				TargetKG:   exercise.TargetKG,
				Tempo:      exercise.Tempo,
			}
		}

		steps := make([]domain.IntervalStep, len(session.IntervalSteps))
		for stepIndex, step := range session.IntervalSteps {
			steps[stepIndex] = domain.IntervalStep{
				Position:   step.Position,
				Label:      step.Label,
				DurationMS: step.DurationMS,
				DistanceM:  step.DistanceM,
			}
		}

		sessions[index] = domain.Session{
			ID:            session.ID,
			Position:      session.Position,
			ScheduledDate: session.ScheduledDate,
			Weekday:       session.Weekday,
			Discipline:    session.Discipline,
			Target:        session.Target,
			Exercises:     exercises,
			Steps:         steps,
		}
	}

	return domain.Plan{
		Reason:          input.Reason,
		EffectiveFrom:   input.EffectiveFrom,
		Timezone:        input.Timezone,
		WeeklyFrequency: input.WeeklyFrequency,
		GoalID:          input.GoalID,
		Sessions:        sessions,
	}
}

type planEnvelope struct {
	Plan planResponse `json:"plan"`
}

type planResponse struct {
	ID              int64             `json:"id"`
	CreatedAt       string            `json:"created_at"`
	Reason          string            `json:"reason"`
	EffectiveFrom   string            `json:"effective_from"`
	Timezone        string            `json:"timezone"`
	WeeklyFrequency int               `json:"weekly_frequency"`
	GoalID          string            `json:"goal_id"`
	Sessions        []sessionResponse `json:"sessions"`
}

type sessionResponse struct {
	ID            string                    `json:"id"`
	Position      int                       `json:"position"`
	ScheduledDate *string                   `json:"scheduled_date"`
	Weekday       *int                      `json:"weekday"`
	Discipline    string                    `json:"discipline"`
	Target        string                    `json:"target"`
	Exercises     []sessionExerciseResponse `json:"exercises"`
	IntervalSteps []intervalStepResponse    `json:"interval_steps"`
}

type sessionExerciseResponse struct {
	Position   int      `json:"position"`
	ExerciseID string   `json:"exercise_id"`
	TargetSets *int     `json:"target_sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	TargetKG   *float64 `json:"target_kg"`
	Tempo      *string  `json:"tempo"`
}

type intervalStepResponse struct {
	Position   int      `json:"position"`
	Label      string   `json:"label"`
	DurationMS *int     `json:"duration_ms"`
	DistanceM  *float64 `json:"distance_m"`
}

func planResponseFrom(plan domain.Plan) planResponse {
	sessions := make([]sessionResponse, len(plan.Sessions))
	for index, session := range plan.Sessions {
		exercises := make([]sessionExerciseResponse, len(session.Exercises))
		for exerciseIndex, exercise := range session.Exercises {
			exercises[exerciseIndex] = sessionExerciseResponse{
				Position:   exercise.Position,
				ExerciseID: exercise.ExerciseID,
				TargetSets: exercise.TargetSets,
				RepsMin:    exercise.RepsMin,
				RepsMax:    exercise.RepsMax,
				TargetKG:   exercise.TargetKG,
				Tempo:      exercise.Tempo,
			}
		}

		steps := make([]intervalStepResponse, len(session.Steps))
		for stepIndex, step := range session.Steps {
			steps[stepIndex] = intervalStepResponse{
				Position:   step.Position,
				Label:      step.Label,
				DurationMS: step.DurationMS,
				DistanceM:  step.DistanceM,
			}
		}

		sessions[index] = sessionResponse{
			ID:            session.ID,
			Position:      session.Position,
			ScheduledDate: session.ScheduledDate,
			Weekday:       session.Weekday,
			Discipline:    session.Discipline,
			Target:        session.Target,
			Exercises:     exercises,
			IntervalSteps: steps,
		}
	}

	return planResponse{
		ID:              plan.ID,
		CreatedAt:       time.UnixMilli(plan.CreatedAt).UTC().Format(time.RFC3339),
		Reason:          plan.Reason,
		EffectiveFrom:   plan.EffectiveFrom,
		Timezone:        plan.Timezone,
		WeeklyFrequency: plan.WeeklyFrequency,
		GoalID:          plan.GoalID,
		Sessions:        sessions,
	}
}
