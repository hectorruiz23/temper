package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"time"

	"github.com/hectorruiz23/temper/api/internal/kernel"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	"github.com/hectorruiz23/temper/api/internal/training/app"
	"github.com/hectorruiz23/temper/api/internal/training/domain"
)

const maxBodyBytes = 64 << 10

var errTrailingBody = errors.New("request body must contain a single JSON object")

type Handler struct {
	service *app.Service
}

func New(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateSession(w nethttp.ResponseWriter, request *nethttp.Request) {
	body, err := readBody(request)
	if err != nil {
		writeBodyError(w, err)
		return
	}

	var decoded createSessionRequest
	if err := decodeJSON(body, &decoded); err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}

	startedAt, err := parseTimestamp(decoded.StartedAt)
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "started_at must be an RFC 3339 date-time")
		return
	}

	session, err := h.service.CreateSession(request.Context(), app.CreateSessionCommand{
		IdempotencyKey: request.Header.Get("Idempotency-Key"),
		ID:             decoded.ID,
		TemplateID:     decoded.TemplateID,
		Date:           decoded.Date,
		Discipline:     decoded.Discipline,
		StartedAt:      startedAt,
	})
	if err != nil {
		writeServiceError(w, err, nethttp.StatusBadRequest)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusCreated, sessionEnvelope{Session: sessionResponseFrom(session)})
}

func (h *Handler) CheckIn(w nethttp.ResponseWriter, request *nethttp.Request) {
	body, err := readBody(request)
	if err != nil {
		writeBodyError(w, err)
		return
	}

	var decoded checkInRequest
	if err := decodeJSON(body, &decoded); err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}

	if decoded.ExpectedRevision == nil {
		httpserver.WriteError(w, nethttp.StatusUnprocessableEntity, "invalid_input", "expected_revision is required")
		return
	}

	reportedAt, err := parseTimestamp(decoded.ReportedCompletedAt)
	if err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "reported_completed_at must be an RFC 3339 date-time")
		return
	}

	result, err := h.service.CheckIn(request.Context(), app.CheckInCommand{
		SessionID:           request.PathValue("id"),
		ExpectedRevision:    *decoded.ExpectedRevision,
		ReportedCompletedAt: reportedAt,
		WatchWorkoutID:      decoded.WatchWorkoutID,
	})
	if err != nil {
		writeServiceError(w, err, nethttp.StatusUnprocessableEntity)
		return
	}

	httpserver.WriteJSON(w, nethttp.StatusOK, checkInEnvelope{
		Session:           sessionResponseFrom(result.Session),
		EffectiveEvidence: evidenceResponseFrom(result.Evidence),
	})
}

func (h *Handler) LogSets(w nethttp.ResponseWriter, request *nethttp.Request) {
	body, err := readBody(request)
	if err != nil {
		writeBodyError(w, err)
		return
	}

	var decoded logSetsRequest
	if err := decodeJSON(body, &decoded); err != nil {
		httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
		return
	}

	sets := make([]domain.NewSet, len(decoded.Sets))
	for index, set := range decoded.Sets {
		if set.KG == nil {
			httpserver.WriteError(w, nethttp.StatusUnprocessableEntity, "invalid_input", "kg is required")
			return
		}
		sets[index] = domain.NewSet{
			ExerciseID: set.ExerciseID,
			SetIndex:   set.SetIndex,
			KG:         *set.KG,
			Reps:       set.Reps,
			Note:       set.Note,
		}
	}

	result, err := h.service.LogSets(request.Context(), app.LogSetsCommand{
		IdempotencyKey: request.Header.Get("Idempotency-Key"),
		SessionID:      decoded.SessionID,
		Sets:           sets,
	})
	if err != nil {
		writeServiceError(w, err, nethttp.StatusUnprocessableEntity)
		return
	}

	echoSets := make([]echoSetResponse, len(result.Echo.Sets))
	for index, set := range result.Echo.Sets {
		echoSets[index] = echoSetResponse{
			ExerciseID: set.ExerciseID,
			SetIndex:   set.SetIndex,
			KG:         set.KG,
			Reps:       set.Reps,
			Note:       set.Note,
		}
	}
	responseSets := make([]setResponse, len(result.Sets))
	for index, set := range result.Sets {
		responseSets[index] = setResponseFrom(set)
	}

	httpserver.WriteJSON(w, nethttp.StatusCreated, batchResponse{
		Sets: responseSets,
		Echo: batchEchoResponse{SessionID: result.Echo.SessionID, Sets: echoSets},
	})
}

func readBody(request *nethttp.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBodyBytes {
		return nil, errBodyTooLarge
	}
	return body, nil
}

var errBodyTooLarge = errors.New("request body exceeds 64 KiB")

func writeBodyError(w nethttp.ResponseWriter, err error) {
	if errors.Is(err, errBodyTooLarge) {
		httpserver.WriteError(w, nethttp.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 64 KiB")
		return
	}
	httpserver.WriteError(w, nethttp.StatusBadRequest, "invalid_input", "invalid request body")
}

func decodeJSON(body []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errTrailingBody
		}
		return err
	}
	return nil
}

func parseTimestamp(value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil, err
	}
	millis := parsed.UnixMilli()
	if millis <= 0 {
		return nil, errors.New("timestamp must be positive")
	}
	return &millis, nil
}

func writeServiceError(w nethttp.ResponseWriter, err error, invalidInputStatus int) {
	var rule kernel.RuleError
	if errors.As(err, &rule) {
		status := statusFor(rule.Code)
		if rule.Code == "invalid_input" {
			status = invalidInputStatus
		}
		httpserver.WriteError(w, status, rule.Code, rule.Message)
		return
	}
	httpserver.WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error")
}

func statusFor(code string) int {
	switch code {
	case "not_found":
		return nethttp.StatusNotFound
	case "revision_conflict", "workout_already_linked", "set_position_occupied", "session_exists", "idempotency_conflict":
		return nethttp.StatusConflict
	case "invalid_input", "idempotency_key_required":
		return nethttp.StatusBadRequest
	case "template_mismatch", "unknown_template", "unusable_watch_evidence", "unknown_exercise":
		return nethttp.StatusUnprocessableEntity
	default:
		return nethttp.StatusInternalServerError
	}
}

type createSessionRequest struct {
	ID         string  `json:"id"`
	TemplateID *string `json:"template_id"`
	Date       string  `json:"date"`
	Discipline string  `json:"discipline"`
	StartedAt  *string `json:"started_at"`
}

type checkInRequest struct {
	ExpectedRevision    *int64  `json:"expected_revision"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
}

type logSetsRequest struct {
	SessionID string       `json:"session_id"`
	Sets      []setRequest `json:"sets"`
}

type setRequest struct {
	ExerciseID string   `json:"exercise_id"`
	SetIndex   int      `json:"set_index"`
	KG         *float64 `json:"kg"`
	Reps       int      `json:"reps"`
	Note       *string  `json:"note"`
}

type sessionEnvelope struct {
	Session sessionResponse `json:"session"`
}

type checkInEnvelope struct {
	Session           sessionResponse   `json:"session"`
	EffectiveEvidence *evidenceResponse `json:"effective_evidence"`
}

type sessionResponse struct {
	ID                  string  `json:"id"`
	TemplateID          *string `json:"template_id"`
	Date                string  `json:"date"`
	Discipline          string  `json:"discipline"`
	StartedAt           *string `json:"started_at"`
	ReportedCompletedAt *string `json:"reported_completed_at"`
	WatchWorkoutID      *string `json:"watch_workout_id"`
	CreatedAt           string  `json:"created_at"`
	Revision            int64   `json:"revision"`
	Complete            bool    `json:"complete"`
}

type evidenceResponse struct {
	Source              string  `json:"source"`
	WorkoutID           *string `json:"workout_id,omitempty"`
	ReportedCompletedAt *string `json:"reported_completed_at,omitempty"`
}

type batchResponse struct {
	Sets []setResponse     `json:"sets"`
	Echo batchEchoResponse `json:"echo"`
}

type batchEchoResponse struct {
	SessionID string            `json:"session_id"`
	Sets      []echoSetResponse `json:"sets"`
}

type echoSetResponse struct {
	ExerciseID string  `json:"exercise_id"`
	SetIndex   int     `json:"set_index"`
	KG         float64 `json:"kg"`
	Reps       int     `json:"reps"`
	Note       *string `json:"note"`
}

type setResponse struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	ExerciseID     string  `json:"exercise_id"`
	SetIndex       int     `json:"set_index"`
	KG             float64 `json:"kg"`
	Reps           int     `json:"reps"`
	Note           *string `json:"note"`
	EvidenceSource string  `json:"evidence_source"`
	CreatedAt      string  `json:"created_at"`
	Revision       int64   `json:"revision"`
}

func sessionResponseFrom(session domain.Session) sessionResponse {
	return sessionResponse{
		ID:                  session.ID,
		TemplateID:          session.TemplateID,
		Date:                session.Date,
		Discipline:          session.Discipline,
		StartedAt:           nullableTimestamp(session.StartedAt),
		ReportedCompletedAt: nullableTimestamp(session.ReportedCompletedAt),
		WatchWorkoutID:      session.WatchWorkoutID,
		CreatedAt:           timestamp(session.CreatedAt),
		Revision:            session.Revision,
		Complete:            session.Complete,
	}
}

func setResponseFrom(set domain.Set) setResponse {
	return setResponse{
		ID:             set.ID,
		SessionID:      set.SessionID,
		ExerciseID:     set.ExerciseID,
		SetIndex:       set.SetIndex,
		KG:             set.KG,
		Reps:           set.Reps,
		Note:           set.Note,
		EvidenceSource: set.EvidenceSource,
		CreatedAt:      timestamp(set.CreatedAt),
		Revision:       set.Revision,
	}
}

func evidenceResponseFrom(evidence *domain.Evidence) *evidenceResponse {
	if evidence == nil {
		return nil
	}
	return &evidenceResponse{
		Source:              evidence.Source,
		WorkoutID:           evidence.WatchWorkoutID,
		ReportedCompletedAt: nullableTimestamp(evidence.ReportedCompletedAt),
	}
}

func timestamp(millis int64) string {
	return time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
}

func nullableTimestamp(millis *int64) *string {
	if millis == nil {
		return nil
	}
	value := timestamp(*millis)
	return &value
}
