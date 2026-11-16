package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"

	"github.com/hectorruiz23/temper/api/internal/health/app"
	"github.com/hectorruiz23/temper/api/internal/health/domain"
	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
)

const maxBodyBytes = 64 << 10

var errTrailingBody = errors.New("request body must contain a single JSON object")

type Handler struct {
	service *app.Service
}

func New(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(w nethttp.ResponseWriter, request *nethttp.Request) {
	key := request.Header.Get("Idempotency-Key")
	if len(key) == 0 || len(key) > 128 {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required and must be at most 128 characters")
		return
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}
	if len(body) > maxBodyBytes {
		httpserver.WriteError(w, nethttp.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 64 KiB")
		return
	}

	decoded, err := decodeDay(body)
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}

	command := app.UpsertDayCommand{
		Date:             decoded.Date,
		Timezone:         decoded.Timezone,
		ExpectedRevision: decoded.ExpectedRevision,
		Source:           decoded.Source,
		IdempotencyKey:   key,
	}
	if decoded.Metrics != nil {
		command.Metrics = domain.MetricsPatch{
			SleepMS:      domain.OptionalInt64{Set: decoded.Metrics.SleepMS.Set, Value: decoded.Metrics.SleepMS.Value},
			Steps:        domain.OptionalInt64{Set: decoded.Metrics.Steps.Set, Value: decoded.Metrics.Steps.Value},
			RestingHRBPM: domain.OptionalInt64{Set: decoded.Metrics.RestingHRBPM.Set, Value: decoded.Metrics.RestingHRBPM.Value},
			HRVRMSSDMS:   domain.OptionalFloat64{Set: decoded.Metrics.HRVRMSSDMS.Set, Value: decoded.Metrics.HRVRMSSDMS.Value},
		}
	}
	if decoded.SleepStages != nil {
		stages := make([]domain.SleepStage, len(*decoded.SleepStages))
		for index, stage := range *decoded.SleepStages {
			stages[index] = domain.SleepStage{Stage: stage.Stage, DurationMS: stage.DurationMS}
		}
		command.SleepStages = &stages
	}
	if decoded.Workouts != nil {
		workouts := make([]domain.Workout, len(*decoded.Workouts))
		for index, workout := range *decoded.Workouts {
			workouts[index] = domain.Workout{
				Origin:        workout.Origin,
				ExternalID:    workout.ExternalID,
				DeviceKind:    workout.DeviceKind,
				StartedAt:     workout.StartedAt,
				EndedAt:       workout.EndedAt,
				Discipline:    workout.Discipline,
				DurationMS:    workout.DurationMS,
				DistanceM:     workout.DistanceM,
				WatchVerified: workout.WatchVerified,
				Present:       true,
			}
		}
		command.Workouts = &workouts
	}

	day, err := h.service.UpsertDay(request.Context(), command)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, dayEnvelope{Day: dayResponseFrom(day)})
}

func (h *Handler) Get(w nethttp.ResponseWriter, request *nethttp.Request) {
	day, err := h.service.GetDay(request.Context(), request.PathValue("date"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, dayEnvelope{Day: dayResponseFrom(day)})
}

func decodeDay(body []byte) (dayRequest, error) {
	var decoded dayRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return dayRequest{}, err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return dayRequest{}, errTrailingBody
		}
		return dayRequest{}, err
	}

	return decoded, nil
}

func writeServiceError(w nethttp.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrDayNotFound) {
		httpserver.WriteError(w, nethttp.StatusNotFound, "not_found", "day not found")
		return
	}

	var rule kernel.RuleError
	if errors.As(err, &rule) {
		status := nethttp.StatusInternalServerError
		switch rule.Code {
		case "invalid_input", "idempotency_key_required":
			status = nethttp.StatusBadRequest
		case "unauthorized":
			status = nethttp.StatusUnauthorized
		case "not_found":
			status = nethttp.StatusNotFound
		case "revision_conflict", "idempotency_conflict":
			status = nethttp.StatusConflict
		case "body_too_large":
			status = nethttp.StatusRequestEntityTooLarge
		}
		httpserver.WriteError(w, status, rule.Code, rule.Message)
		return
	}

	httpserver.WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error")
}

type dayRequest struct {
	Date             string            `json:"date"`
	Timezone         string            `json:"timezone"`
	ExpectedRevision int64             `json:"expected_revision"`
	Source           string            `json:"source"`
	Metrics          *metricsRequest   `json:"metrics"`
	SleepStages      *[]stageRequest   `json:"sleep_stages"`
	Workouts         *[]workoutRequest `json:"workouts"`
}

type metricsRequest struct {
	SleepMS      optionalInt64   `json:"sleep_ms"`
	Steps        optionalInt64   `json:"steps"`
	RestingHRBPM optionalInt64   `json:"resting_hr_bpm"`
	HRVRMSSDMS   optionalFloat64 `json:"hrv_rmssd_ms"`
}

type optionalInt64 struct {
	Set   bool
	Value *int64
}

func (o *optionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type optionalFloat64 struct {
	Set   bool
	Value *float64
}

func (o *optionalFloat64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type stageRequest struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}

type workoutRequest struct {
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

type dayEnvelope struct {
	Day dayResponse `json:"day"`
}

type dayResponse struct {
	Date         string            `json:"date"`
	Timezone     string            `json:"timezone"`
	Revision     int64             `json:"revision"`
	SleepMS      *int64            `json:"sleep_ms"`
	Steps        *int64            `json:"steps"`
	RestingHRBPM *int64            `json:"resting_hr_bpm"`
	HRVRMSSDMS   *float64          `json:"hrv_rmssd_ms"`
	UpdatedAt    int64             `json:"updated_at"`
	LastSource   string            `json:"last_source"`
	SleepStages  []stageResponse   `json:"sleep_stages"`
	Workouts     []workoutResponse `json:"workouts"`
}

type stageResponse struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}

type workoutResponse struct {
	ID            string   `json:"id"`
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

func dayResponseFrom(day domain.Day) dayResponse {
	stages := make([]stageResponse, len(day.SleepStages))
	for index, stage := range day.SleepStages {
		stages[index] = stageResponse{Stage: stage.Stage, DurationMS: stage.DurationMS}
	}

	workouts := make([]workoutResponse, 0, len(day.Workouts))
	for _, workout := range day.Workouts {
		if !workout.Present {
			continue
		}
		workouts = append(workouts, workoutResponse{
			ID:            workout.ID,
			Origin:        workout.Origin,
			ExternalID:    workout.ExternalID,
			DeviceKind:    workout.DeviceKind,
			StartedAt:     workout.StartedAt,
			EndedAt:       workout.EndedAt,
			Discipline:    workout.Discipline,
			DurationMS:    workout.DurationMS,
			DistanceM:     workout.DistanceM,
			WatchVerified: workout.WatchVerified,
		})
	}

	return dayResponse{
		Date:         day.Date,
		Timezone:     day.Timezone,
		Revision:     day.Revision,
		SleepMS:      day.SleepMS,
		Steps:        day.Steps,
		RestingHRBPM: day.RestingHRBPM,
		HRVRMSSDMS:   day.HRVRMSSDMS,
		UpdatedAt:    day.UpdatedAt,
		LastSource:   day.LastSource,
		SleepStages:  stages,
		Workouts:     workouts,
	}
}
